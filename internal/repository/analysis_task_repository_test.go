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
)

func TestClaimNextPendingChangesTaskToRunning(t *testing.T) {
	db, mock := mockMySQLRepository(t)
	repo := NewAnalysisTaskRepository(db)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT *\nFROM analysis_tasks\nWHERE status = ?\nORDER BY created_at, id\nLIMIT 1\nFOR UPDATE SKIP LOCKED")).
		WithArgs(model.AnalysisTaskStatusPending).
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

func TestMarkSucceededWritesResultJSONAndUsesRunningCondition(t *testing.T) {
	db, mock := mockMySQLRepository(t)
	repo := NewAnalysisTaskRepository(db)
	reportID := uint(15)
	wantJSON := `{"report_id":15,"degraded":false}`
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE ` + "`analysis_tasks`" + ` SET .*` + "`result_json`" + `=\?.* WHERE id = \? AND status = \?`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := repo.MarkSucceeded(context.Background(), 21, wantJSON, &reportID, false); err != nil {
		t.Fatalf("MarkSucceeded() error = %v", err)
	}
	assertSQLExpectations(t, mock)
}

func TestRecoverStaleTasksUsesCutoffAndLeavesNonStaleRowsUntouched(t *testing.T) {
	db, mock := mockMySQLRepository(t)
	repo := NewAnalysisTaskRepository(db)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE ` + "`analysis_tasks`" + ` SET .* WHERE .*started_at < \?.*attempt_count < max_attempts`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE ` + "`analysis_tasks`" + ` SET .* WHERE .*started_at < \?.*attempt_count >= max_attempts`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	result, err := repo.RecoverStaleTasks(context.Background(), time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if result.Pending != 1 || result.Failed != 0 {
		t.Fatalf("unexpected recovery result: %#v", result)
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
