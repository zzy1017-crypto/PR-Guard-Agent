package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"pr-guard-agent/internal/config"
	"pr-guard-agent/internal/model"
	"pr-guard-agent/internal/repository"
	"pr-guard-agent/internal/runtimeinfo"
)

type fakeAnalysisTaskOpsRepository struct {
	tasks       []model.AnalysisTask
	total       int64
	metrics     repository.TaskMetrics
	errors      []repository.ErrorCodeMetric
	lastFilter  repository.TaskListFilter
	lastSince   time.Time
	lastStale   time.Time
	errorsLimit int
}

func (r *fakeAnalysisTaskOpsRepository) ListTasks(_ context.Context, filter repository.TaskListFilter) ([]model.AnalysisTask, int64, error) {
	r.lastFilter = filter
	return r.tasks, r.total, nil
}

func (r *fakeAnalysisTaskOpsRepository) GetTaskMetrics(_ context.Context, since time.Time, staleBefore time.Time) (*repository.TaskMetrics, error) {
	r.lastSince = since
	r.lastStale = staleBefore
	copy := r.metrics
	return &copy, nil
}

func (r *fakeAnalysisTaskOpsRepository) GetErrorsByCode(_ context.Context, since time.Time, limit int) ([]repository.ErrorCodeMetric, error) {
	r.lastSince = since
	r.errorsLimit = limit
	return r.errors, nil
}

type fakeWorkerRuntimeProvider struct {
	snapshot runtimeinfo.WorkerRuntimeSnapshot
}

func (p fakeWorkerRuntimeProvider) Snapshot() runtimeinfo.WorkerRuntimeSnapshot {
	return p.snapshot
}

func TestAnalysisTaskOpsListReturnsOnlySafeFields(t *testing.T) {
	nextRunAt := time.Now().Add(time.Minute)
	repo := &fakeAnalysisTaskOpsRepository{
		total: 1,
		tasks: []model.AnalysisTask{{
			ID: 7, TaskKey: "secret-key", ProjectID: 2, DiffID: 3, TopK: 5,
			Status: model.AnalysisTaskStatusPending, AttemptCount: 1, MaxAttempts: 3,
			ResultJSON:   `{"full_report":"must not leak"}`,
			ErrorMessage: "safe message\nsecond line", NextRunAt: &nextRunAt,
		}},
	}
	service := NewAnalysisTaskOpsService(repo, testOpsConfig(), testOpsWorkerConfig(), fakeWorkerRuntimeProvider{}, nil)
	filter := repository.TaskListFilter{Page: 2, PageSize: 10}
	result, err := service.ListTasks(context.Background(), filter)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || result.Page != 2 || result.PageSize != 10 || len(result.Items) != 1 {
		t.Fatalf("unexpected list result: %#v", result)
	}
	if !result.Items[0].RetryScheduled || result.Items[0].ErrorMessage != "safe message second line" {
		t.Fatalf("unexpected item: %#v", result.Items[0])
	}
	body, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) == "" || containsAny(string(body), "result_json", "full_report", "task_key") {
		t.Fatalf("sensitive field leaked: %s", body)
	}
}

func TestAnalysisTaskOpsMetricsZeroDenominatorsAndCutoffs(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	repo := &fakeAnalysisTaskOpsRepository{
		metrics: repository.TaskMetrics{
			PendingCount: 3, DuePendingCount: 2, ScheduledRetryCount: 1,
			RunningCount: 2, StaleRunningCount: 1, OldestPendingAgeSeconds: 90,
		},
		errors: []repository.ErrorCodeMetric{{ErrorCode: "unknown", Count: 2}},
	}
	service := NewAnalysisTaskOpsService(repo, testOpsConfig(), testOpsWorkerConfig(), fakeWorkerRuntimeProvider{}, nil)
	service.now = func() time.Time { return now }
	result, err := service.GetMetrics(context.Background(), 24)
	if err != nil {
		t.Fatal(err)
	}
	if result.Window.SuccessRatePercent != 0 || result.Window.FailureRatePercent != 0 ||
		result.Window.DegradedRatePercent != 0 || result.Window.RetryRatePercent != 0 {
		t.Fatalf("zero denominator returned non-zero rates: %#v", result.Window)
	}
	if result.Current.DuePendingCount+result.Current.ScheduledRetryCount != result.Current.PendingCount {
		t.Fatalf("due and scheduled counts overlap or omit test snapshot: %#v", result.Current)
	}
	if !repo.lastSince.Equal(now.Add(-24 * time.Hour)) {
		t.Fatalf("since = %v", repo.lastSince)
	}
	if !repo.lastStale.Equal(now.Add(-60 * time.Second)) {
		t.Fatalf("staleBefore = %v", repo.lastStale)
	}
	if repo.errorsLimit != 20 || len(result.ErrorsByCode) != 1 {
		t.Fatalf("unexpected error distribution: %#v", result.ErrorsByCode)
	}
}

func TestAnalysisTaskOpsMetricsPercentages(t *testing.T) {
	repo := &fakeAnalysisTaskOpsRepository{metrics: repository.TaskMetrics{
		SubmittedCount: 10, SucceededCount: 6, FailedCount: 2,
		DegradedSucceededCount: 3, RetriedTaskCount: 4,
	}}
	service := NewAnalysisTaskOpsService(repo, testOpsConfig(), testOpsWorkerConfig(), fakeWorkerRuntimeProvider{}, nil)
	result, err := service.GetMetrics(context.Background(), 24)
	if err != nil {
		t.Fatal(err)
	}
	if result.Window.SuccessRatePercent != 75 || result.Window.FailureRatePercent != 25 ||
		result.Window.DegradedRatePercent != 50 || result.Window.RetryRatePercent != 40 {
		t.Fatalf("unexpected rates: %#v", result.Window)
	}
}

func TestAnalysisTaskOpsWorkerStatus(t *testing.T) {
	startedAt := time.Now().Add(-10 * time.Second)
	provider := fakeWorkerRuntimeProvider{snapshot: runtimeinfo.WorkerRuntimeSnapshot{
		StartedAt: startedAt,
		Workers: []runtimeinfo.WorkerRuntime{
			{WorkerID: "worker-1", Busy: true, CurrentTaskID: 9, ProcessedCount: 3, SuccessCount: 2, FailureCount: 1},
			{WorkerID: "worker-2"},
		},
	}}
	service := NewAnalysisTaskOpsService(nil, testOpsConfig(), testOpsWorkerConfig(), provider, func() bool { return true })
	service.now = func() time.Time { return startedAt.Add(10 * time.Second) }
	result, err := service.GetWorkers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.ManagerEnabled || result.ConfiguredWorkerCount != 2 || result.RegisteredWorkerCount != 2 ||
		result.BusyWorkerCount != 1 || result.IdleWorkerCount != 1 || result.UptimeSeconds != 10 ||
		!result.RuntimeMetricsResetOnRestart || !result.Stopping {
		t.Fatalf("unexpected worker status: %#v", result)
	}
}

func testOpsConfig() config.OpsConfig {
	return config.OpsConfig{
		Enabled: true, DefaultPageSize: 20, MaxPageSize: 100,
		DefaultMetricsWindowHours: 24, MaxMetricsWindowHours: 168,
		QueryTimeoutSeconds: 3,
	}
}

func testOpsWorkerConfig() config.AnalysisWorkerConfig {
	return config.AnalysisWorkerConfig{
		Enabled: true, WorkerCount: 2, PollIntervalMS: 10,
		TaskTimeoutSeconds: 10, StaleAfterSeconds: 60, MaxAttempts: 3,
		RetryBaseSeconds: 1, RetryMaxSeconds: 30,
	}
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
