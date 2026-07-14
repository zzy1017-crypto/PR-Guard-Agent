package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"pr-guard-agent/internal/model"
	"pr-guard-agent/internal/taskerror"
)

func TestClaimNextPendingChangesTaskToRunning(t *testing.T) {
	db, mock := mockMySQLRepository(t)
	repo := NewAnalysisTaskRepository(db)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT *\nFROM analysis_tasks\nWHERE status = ?\nAND (next_run_at IS NULL OR next_run_at <= ?)\nORDER BY created_at, id\nLIMIT 1\nFOR UPDATE SKIP LOCKED")).
		WithArgs(model.AnalysisTaskStatusPending, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_key", "project_id", "diff_id", "top_k", "status", "attempt_count", "max_attempts", "created_at"}).
			AddRow(21, "task-key", 1, 2, 5, model.AnalysisTaskStatusPending, 0, 3, time.Now()))
	mock.ExpectExec("UPDATE `analysis_tasks` SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	task, err := repo.ClaimNextPending(context.Background(), "worker-1")
	if err != nil {
		t.Fatalf("ClaimNextPending() error = %v", err)
	}
	if task.ID != 21 || task.Status != model.AnalysisTaskStatusRunning || task.AttemptCount != 1 || task.WorkerID != "worker-1" || task.StartedAt == nil {
		t.Fatalf("unexpected claimed task: %#v", task)
	}
	assertSQLExpectations(t, mock)
}

func TestSecondWorkerCannotClaimSamePendingTask(t *testing.T) {
	db, mock := mockMySQLRepository(t)
	repo := NewAnalysisTaskRepository(db)

	// The first transaction claims the only row. A subsequent worker sees no
	// pending row; in MySQL concurrent execution gets the same result because
	// the locked row is skipped by SKIP LOCKED.
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \*`).WillReturnRows(sqlmock.NewRows([]string{"id", "status", "attempt_count", "created_at"}).
		AddRow(1, model.AnalysisTaskStatusPending, 0, time.Now()))
	mock.ExpectExec("UPDATE `analysis_tasks` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if _, err := repo.ClaimNextPending(context.Background(), "worker-1"); err != nil {
		t.Fatal(err)
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \*`).WillReturnRows(sqlmock.NewRows([]string{"id", "status"}))
	mock.ExpectRollback()
	if _, err := repo.ClaimNextPending(context.Background(), "worker-2"); !errors.Is(err, ErrNoPendingTask) {
		t.Fatalf("second claim error = %v, want ErrNoPendingTask", err)
	}
	assertSQLExpectations(t, mock)
}

func TestClaimNextPendingDoesNotClaimBeforeNextRunAt(t *testing.T) {
	db, mock := mockMySQLRepository(t)
	repo := NewAnalysisTaskRepository(db)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \*`).
		WithArgs(model.AnalysisTaskStatusPending, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "next_run_at"}))
	mock.ExpectRollback()

	if _, err := repo.ClaimNextPending(context.Background(), "worker-1"); !errors.Is(err, ErrNoPendingTask) {
		t.Fatalf("ClaimNextPending() error = %v, want ErrNoPendingTask", err)
	}
	assertSQLExpectations(t, mock)
}

func TestClaimNextPendingClaimsTaskAtNextRunAt(t *testing.T) {
	db, mock := mockMySQLRepository(t)
	repo := NewAnalysisTaskRepository(db)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \*`).
		WithArgs(model.AnalysisTaskStatusPending, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "attempt_count", "max_attempts", "next_run_at", "created_at"}).
			AddRow(25, model.AnalysisTaskStatusPending, 1, 3, time.Now().Add(-time.Millisecond), time.Now()))
	mock.ExpectExec("UPDATE `analysis_tasks` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	task, err := repo.ClaimNextPending(context.Background(), "worker-2")
	if err != nil {
		t.Fatal(err)
	}
	if task.AttemptCount != 2 || task.NextRunAt != nil || task.Status != model.AnalysisTaskStatusRunning {
		t.Fatalf("unexpected claimed task: %#v", task)
	}
	assertSQLExpectations(t, mock)
}

func TestMarkSucceededWritesResultJSONAndUsesRunningCondition(t *testing.T) {
	db, mock := mockMySQLRepository(t)
	repo := NewAnalysisTaskRepository(db)
	reportID := uint(15)
	wantJSON := `{"report_id":15,"degraded":false}`
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE ` + "`analysis_tasks`" + ` SET .*` + "`error_code`" + `=\?.*` + "`next_run_at`" + `=\?.* WHERE id = \? AND status = \? AND worker_id = \? AND attempt_count = \?`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := repo.MarkSucceeded(context.Background(), 21, "worker-1", 2, wantJSON, &reportID, false); err != nil {
		t.Fatalf("MarkSucceeded() error = %v", err)
	}
	assertSQLExpectations(t, mock)
}

func TestRequeueWithBackoffReturnsConflictWhenStateDoesNotMatch(t *testing.T) {
	db, mock := mockMySQLRepository(t)
	repo := NewAnalysisTaskRepository(db)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE ` + "`analysis_tasks`" + ` SET .* WHERE id = \? AND status = \? AND worker_id = \? AND attempt_count = \?`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	err := repo.RequeueWithBackoff(context.Background(), 21, "worker-1", 2, time.Now().Add(time.Second), "qdrant_unavailable", "code retrieval service is temporarily unavailable")
	if !errors.Is(err, ErrTaskStateConflict) {
		t.Fatalf("RequeueWithBackoff() error = %v, want ErrTaskStateConflict", err)
	}
	assertSQLExpectations(t, mock)
}

func TestRecoverStaleTasksUsesCutoffAndLeavesNonStaleRowsUntouched(t *testing.T) {
	db, mock := mockMySQLRepository(t)
	repo := NewAnalysisTaskRepository(db)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, worker_id, attempt_count, max_attempts`).
		WithArgs(model.AnalysisTaskStatusRunning, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "worker_id", "attempt_count", "max_attempts"}).
			AddRow(31, "stale-worker-1", 1, 3).
			AddRow(32, "stale-worker-2", 3, 3))
	mock.ExpectExec(`UPDATE ` + "`analysis_tasks`" + ` SET .* WHERE id = \? AND status = \? AND worker_id = \? AND attempt_count = \? AND started_at < \?`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE ` + "`analysis_tasks`" + ` SET .* WHERE id = \? AND status = \? AND worker_id = \? AND attempt_count = \? AND started_at < \?`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := repo.RecoverStaleTasks(context.Background(), time.Now().Add(-time.Hour), taskerror.RetryPolicy{
		BaseDelay: 2 * time.Second,
		MaxDelay:  30 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Pending != 1 || result.Failed != 1 || len(result.Scheduled) != 1 || len(result.Exhausted) != 1 {
		t.Fatalf("unexpected recovery result: %#v", result)
	}
	if result.Scheduled[0].NextRunAt == nil || result.Scheduled[0].NextRunAt.Before(time.Now().Add(time.Second)) {
		t.Fatalf("stale retry did not receive backoff: %#v", result.Scheduled[0])
	}
	assertSQLExpectations(t, mock)
}

func mockMySQLRepository(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	return db, mock
}

func assertSQLExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
