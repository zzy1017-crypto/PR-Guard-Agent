package repository

import (
	"context"
	"database/sql/driver"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"pr-guard-agent/internal/model"
)

func TestListTasksAppliesFiltersCountsSeparatelyAndOmitsResultJSON(t *testing.T) {
	db, mock := mockMySQLRepository(t)
	repo := NewAnalysisTaskRepository(db)
	status := model.AnalysisTaskStatusFailed
	projectID := uint(2)
	diffID := uint(3)
	errorCode := "qdrant_unavailable"
	degraded := false
	createdFrom := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	createdTo := createdFrom.Add(24 * time.Hour)
	filter := TaskListFilter{
		Status: &status, ProjectID: &projectID, DiffID: &diffID,
		ErrorCode: &errorCode, Degraded: &degraded,
		CreatedFrom: &createdFrom, CreatedTo: &createdTo,
		Page: 2, PageSize: 10,
	}
	args := []driver.Value{status, projectID, diffID, errorCode, degraded, createdFrom, createdTo}
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `analysis_tasks` WHERE").
		WithArgs(args...).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(11))
	mock.ExpectQuery("SELECT .*`update_at` FROM `analysis_tasks` WHERE .*ORDER BY created_at DESC,id DESC LIMIT \\? OFFSET \\?").
		WithArgs(append(args, 10, 10)...).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "project_id", "diff_id", "top_k", "status", "attempt_count",
			"max_attempts", "worker_id", "report_id", "degraded", "error_code",
			"error_message", "next_run_at", "started_at", "finished_at", "created_at", "update_at",
		}).AddRow(
			7, 2, 3, 5, status, 3, 3, "worker-1", nil, false,
			errorCode, "safe", nil, createdFrom, createdTo, createdFrom, createdTo,
		))

	tasks, total, err := repo.ListTasks(context.Background(), filter)
	if err != nil {
		t.Fatal(err)
	}
	if total != 11 || len(tasks) != 1 || tasks[0].ID != 7 {
		t.Fatalf("unexpected list result total=%d tasks=%#v", total, tasks)
	}
	if tasks[0].ResultJSON != "" || tasks[0].TaskKey != "" || tasks[0].SubmitRequestID != "" {
		t.Fatalf("unselected sensitive fields were populated: %#v", tasks[0])
	}
	assertSQLExpectations(t, mock)
}

func TestGetTaskMetricsUsesDisjointPendingAndDatabaseAggregates(t *testing.T) {
	db, mock := mockMySQLRepository(t)
	repo := NewAnalysisTaskRepository(db)
	since := time.Now().Add(-24 * time.Hour)
	staleBefore := time.Now().Add(-3 * time.Minute)

	mock.ExpectQuery(regexp.QuoteMeta("status = ? AND (next_run_at IS NULL OR next_run_at <= NOW())")+".*"+
		regexp.QuoteMeta("status = ? AND attempt_count > 0 AND next_run_at > NOW()")).
		WithArgs(
			model.AnalysisTaskStatusPending,
			model.AnalysisTaskStatusPending,
			model.AnalysisTaskStatusPending,
			model.AnalysisTaskStatusRunning,
			model.AnalysisTaskStatusRunning,
			staleBefore,
			model.AnalysisTaskStatusPending,
		).
		WillReturnRows(sqlmock.NewRows([]string{
			"pending_count", "due_pending_count", "scheduled_retry_count", "running_count",
			"stale_running_count", "oldest_pending_age_seconds",
		}).AddRow(4, 3, 1, 2, 1, 120))
	mock.ExpectQuery("COUNT\\(\\*\\) AS submitted_count.*TIMESTAMPDIFF\\(MICROSECOND, created_at, started_at\\).*WHERE created_at >= \\?").
		WithArgs(
			model.AnalysisTaskStatusSucceeded,
			model.AnalysisTaskStatusFailed,
			model.AnalysisTaskStatusPending,
			model.AnalysisTaskStatusRunning,
			model.AnalysisTaskStatusSucceeded,
			model.AnalysisTaskStatusSucceeded,
			model.AnalysisTaskStatusFailed,
			model.AnalysisTaskStatusSucceeded,
			model.AnalysisTaskStatusFailed,
			since,
		).
		WillReturnRows(sqlmock.NewRows([]string{
			"submitted_count", "succeeded_count", "failed_count", "unfinished_count",
			"degraded_succeeded_count", "retried_task_count", "avg_queue_wait_ms",
			"avg_run_duration_ms", "max_run_duration_ms",
		}).AddRow(10, 6, 2, 2, 3, 4, 125.5, 800.25, 1500.0))

	metrics, err := repo.GetTaskMetrics(context.Background(), since, staleBefore)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.DuePendingCount != 3 || metrics.ScheduledRetryCount != 1 ||
		metrics.StaleRunningCount != 1 || metrics.SubmittedCount != 10 ||
		metrics.AvgRunDurationMS != 800.25 {
		t.Fatalf("unexpected metrics: %#v", metrics)
	}
	assertSQLExpectations(t, mock)
}

func TestGetErrorsByCodeGroupsUnknownAndLimitsResults(t *testing.T) {
	db, mock := mockMySQLRepository(t)
	repo := NewAnalysisTaskRepository(db)
	since := time.Now().Add(-24 * time.Hour)
	mock.ExpectQuery("SELECT COALESCE\\(NULLIF\\(error_code, ''\\), 'unknown'\\) AS error_code, COUNT\\(\\*\\) AS count FROM `analysis_tasks` WHERE created_at >= \\? AND status = \\? GROUP BY .* ORDER BY count DESC,error_code ASC LIMIT \\?").
		WithArgs(since, model.AnalysisTaskStatusFailed, 20).
		WillReturnRows(sqlmock.NewRows([]string{"error_code", "count"}).
			AddRow("qdrant_unavailable", 4).
			AddRow("unknown", 2))

	metrics, err := repo.GetErrorsByCode(context.Background(), since, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics) != 2 || metrics[0].ErrorCode != "qdrant_unavailable" || metrics[0].Count != 4 ||
		metrics[1].ErrorCode != "unknown" {
		t.Fatalf("unexpected errors by code: %#v", metrics)
	}
	assertSQLExpectations(t, mock)
}
