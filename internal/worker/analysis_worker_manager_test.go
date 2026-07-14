package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"pr-guard-agent/internal/config"
	"pr-guard-agent/internal/model"
	"pr-guard-agent/internal/repository"
	"pr-guard-agent/internal/service"
	"pr-guard-agent/internal/taskerror"
)

type analyzerFunc func(context.Context, uint, uint, int) (*service.AnalyzeResult, error)

func (f analyzerFunc) AnalyzeDiff(ctx context.Context, projectID uint, diffID uint, topK int) (*service.AnalyzeResult, error) {
	return f(ctx, projectID, diffID, topK)
}

func TestProcessTaskMarksFallbackSucceededAndDegraded(t *testing.T) {
	repo, task := runningWorkerTask("fallback-key")
	analyzer := analyzerFunc(func(context.Context, uint, uint, int) (*service.AnalyzeResult, error) {
		return &service.AnalyzeResult{
			ProjectID: task.ProjectID, DiffID: task.DiffID,
			RiskLevel: "medium", Summary: "fallback", Degraded: true,
		}, nil
	})
	manager := NewAnalysisWorkerManager(repo, analyzer, testWorkerConfig(), nil)
	manager.processTask(task, "worker-test")

	got := repo.load()
	if got.Status != model.AnalysisTaskStatusSucceeded || !got.Degraded || got.ReportID != nil || got.ResultJSON == "" {
		t.Fatalf("unexpected fallback task: %#v", got)
	}
	var result service.AnalyzeResult
	if err := json.Unmarshal([]byte(got.ResultJSON), &result); err != nil || !result.Degraded {
		t.Fatalf("invalid fallback result JSON: %q, error=%v", got.ResultJSON, err)
	}
	if repo.requeueCalls != 0 || repo.failedCalls != 0 {
		t.Fatalf("fallback entered failure path: requeue=%d failed=%d", repo.requeueCalls, repo.failedCalls)
	}
}

func TestProcessTaskUnknownFailureIsPermanent(t *testing.T) {
	repo, task := runningWorkerTask("failure-key")
	analyzer := analyzerFunc(func(context.Context, uint, uint, int) (*service.AnalyzeResult, error) {
		return nil, errors.New("provider internal response must not be stored")
	})
	manager := NewAnalysisWorkerManager(repo, analyzer, testWorkerConfig(), nil)
	manager.processTask(task, "worker-test")

	got := repo.load()
	if got.Status != model.AnalysisTaskStatusFailed || got.ErrorCode != taskerror.CodeInternalAnalysisError || got.ErrorMessage != "analysis task failed" || got.FinishedAt == nil {
		t.Fatalf("unexpected failed task: %#v", got)
	}
	if strings.Contains(got.ErrorMessage, "provider") {
		t.Fatalf("unsafe provider error was persisted: %q", got.ErrorMessage)
	}
}

func TestProcessTaskRetryableFailureSchedulesBackoff(t *testing.T) {
	repo, task := runningWorkerTask("retry-key")
	analyzer := analyzerFunc(func(context.Context, uint, uint, int) (*service.AnalyzeResult, error) {
		return nil, fmt.Errorf("retrieve failed: %w", taskerror.ErrQdrantUnavailable)
	})
	manager := NewAnalysisWorkerManager(repo, analyzer, testWorkerConfig(), nil)
	before := time.Now()
	manager.processTask(task, "worker-test")

	got := repo.load()
	if got.Status != model.AnalysisTaskStatusPending || got.NextRunAt == nil || !got.NextRunAt.After(before) {
		t.Fatalf("retry was not scheduled: %#v", got)
	}
	if got.ErrorCode != taskerror.CodeQdrantUnavailable || got.FinishedAt != nil || repo.requeueCalls != 1 {
		t.Fatalf("unexpected retry state: %#v", got)
	}
}

func TestProcessTaskNonRetryableFailureIsFinal(t *testing.T) {
	repo, task := runningWorkerTask("permanent-key")
	analyzer := analyzerFunc(func(context.Context, uint, uint, int) (*service.AnalyzeResult, error) {
		return nil, service.ErrProjectNotFound
	})
	manager := NewAnalysisWorkerManager(repo, analyzer, testWorkerConfig(), nil)
	manager.processTask(task, "worker-test")

	got := repo.load()
	if got.Status != model.AnalysisTaskStatusFailed || got.ErrorCode != taskerror.CodeProjectNotFound || repo.requeueCalls != 0 {
		t.Fatalf("unexpected permanent failure: %#v", got)
	}
}

func TestProcessTaskRetryExhaustedIsFinal(t *testing.T) {
	repo, task := runningWorkerTask("exhausted-key")
	task.AttemptCount = task.MaxAttempts
	analyzer := analyzerFunc(func(context.Context, uint, uint, int) (*service.AnalyzeResult, error) {
		return nil, taskerror.ErrQdrantUnavailable
	})
	manager := NewAnalysisWorkerManager(repo, analyzer, testWorkerConfig(), nil)
	manager.processTask(task, "worker-test")

	got := repo.load()
	if got.Status != model.AnalysisTaskStatusFailed || got.ErrorCode != taskerror.CodeRetryExhausted || repo.requeueCalls != 0 {
		t.Fatalf("unexpected exhausted failure: %#v", got)
	}
	if !strings.Contains(got.ErrorMessage, "maximum attempts") || strings.Contains(got.ErrorMessage, "provider response") {
		t.Fatalf("unexpected exhausted message: %q", got.ErrorMessage)
	}
}

func TestProcessTaskSuccessClearsPreviousError(t *testing.T) {
	repo, task := runningWorkerTask("success-cleanup-key")
	nextRunAt := time.Now()
	task.NextRunAt = &nextRunAt
	task.ErrorCode = taskerror.CodeQdrantUnavailable
	task.ErrorMessage = "code retrieval service is temporarily unavailable"
	analyzer := analyzerFunc(func(context.Context, uint, uint, int) (*service.AnalyzeResult, error) {
		return &service.AnalyzeResult{ProjectID: 1, DiffID: 2, Summary: "ok"}, nil
	})
	manager := NewAnalysisWorkerManager(repo, analyzer, testWorkerConfig(), nil)
	manager.processTask(task, "worker-test")

	got := repo.load()
	if got.Status != model.AnalysisTaskStatusSucceeded || got.NextRunAt != nil || got.ErrorCode != "" || got.ErrorMessage != "" {
		t.Fatalf("success did not clear retry error: %#v", got)
	}
}

type memoryWorkerRepository struct {
	mu           sync.Mutex
	task         *model.AnalysisTask
	requeueCalls int
	failedCalls  int
}

func (r *memoryWorkerRepository) ClaimNextPending(context.Context, string) (*model.AnalysisTask, error) {
	return nil, repository.ErrNoPendingTask
}

func (r *memoryWorkerRepository) MarkSucceeded(_ context.Context, id uint64, workerID string, expectedAttempt int, resultJSON string, reportID *uint, degraded bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.task.ID != id || r.task.Status != model.AnalysisTaskStatusRunning || r.task.AttemptCount != expectedAttempt {
		return repository.ErrTaskStateChanged
	}
	now := time.Now()
	r.task.Status, r.task.ResultJSON, r.task.ReportID = model.AnalysisTaskStatusSucceeded, resultJSON, reportID
	r.task.Degraded, r.task.FinishedAt = degraded, &now
	r.task.ErrorCode, r.task.ErrorMessage = "", ""
	r.task.NextRunAt = nil
	return nil
}

func (r *memoryWorkerRepository) RequeueWithBackoff(_ context.Context, id uint64, workerID string, expectedAttempt int, nextRunAt time.Time, code, message string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.task.ID != id || r.task.Status != model.AnalysisTaskStatusRunning || r.task.AttemptCount != expectedAttempt {
		return repository.ErrTaskStateChanged
	}
	now := time.Now()
	r.requeueCalls++
	r.task.Status, r.task.ErrorCode, r.task.ErrorMessage = model.AnalysisTaskStatusPending, code, message
	r.task.WorkerID, r.task.StartedAt, r.task.FinishedAt = "", nil, nil
	r.task.NextRunAt, r.task.LastFailedAt = &nextRunAt, &now
	return nil
}

func (r *memoryWorkerRepository) MarkFailedFinal(_ context.Context, id uint64, workerID string, expectedAttempt int, code, message string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.task.ID != id || r.task.Status != model.AnalysisTaskStatusRunning || r.task.AttemptCount != expectedAttempt {
		return repository.ErrTaskStateChanged
	}
	now := time.Now()
	r.failedCalls++
	r.task.Status, r.task.ErrorCode, r.task.ErrorMessage = model.AnalysisTaskStatusFailed, code, message
	r.task.FinishedAt, r.task.LastFailedAt, r.task.NextRunAt = &now, &now, nil
	return nil
}

func (r *memoryWorkerRepository) RecoverStaleTasks(context.Context, time.Time, taskerror.RetryPolicy) (repository.StaleRecoveryResult, error) {
	return repository.StaleRecoveryResult{}, nil
}

func (r *memoryWorkerRepository) load() *model.AnalysisTask {
	r.mu.Lock()
	defer r.mu.Unlock()
	copy := *r.task
	return &copy
}

func runningWorkerTask(key string) (*memoryWorkerRepository, *model.AnalysisTask) {
	task := &model.AnalysisTask{
		ID: 1, TaskKey: key, ProjectID: 1, DiffID: 2, TopK: 5,
		Status: model.AnalysisTaskStatusRunning, AttemptCount: 1, MaxAttempts: 3, WorkerID: "worker-test",
	}
	return &memoryWorkerRepository{task: task}, task
}

func testWorkerConfig() config.AnalysisWorkerConfig {
	return config.AnalysisWorkerConfig{
		Enabled: true, WorkerCount: 1, PollIntervalMS: 10,
		TaskTimeoutSeconds: 1, StaleAfterSeconds: 60, MaxAttempts: 3,
		RetryBaseSeconds: 1, RetryMaxSeconds: 30, RetryJitterPercent: 0,
	}
}
