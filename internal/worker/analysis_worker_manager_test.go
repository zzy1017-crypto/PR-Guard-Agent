package worker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"pr-guard-agent/internal/config"
	"pr-guard-agent/internal/model"
	"pr-guard-agent/internal/repository"
	"pr-guard-agent/internal/service"
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
}

func TestProcessTaskMarksAnalyzeFailureFailed(t *testing.T) {
	repo, task := runningWorkerTask("failure-key")
	analyzer := analyzerFunc(func(context.Context, uint, uint, int) (*service.AnalyzeResult, error) {
		return nil, errors.New("provider internal response must not be stored")
	})
	manager := NewAnalysisWorkerManager(repo, analyzer, testWorkerConfig(), nil)
	manager.processTask(task, "worker-test")

	got := repo.load()
	if got.Status != model.AnalysisTaskStatusFailed || got.ErrorCode != "analysis_failed" || got.ErrorMessage != "analysis task failed" || got.FinishedAt == nil {
		t.Fatalf("unexpected failed task: %#v", got)
	}
	if strings.Contains(got.ErrorMessage, "provider") {
		t.Fatalf("unsafe provider error was persisted: %q", got.ErrorMessage)
	}
}

type memoryWorkerRepository struct {
	mu   sync.Mutex
	task *model.AnalysisTask
}

func (r *memoryWorkerRepository) ClaimNextPending(context.Context, string) (*model.AnalysisTask, error) {
	return nil, repository.ErrNoPendingTask
}

func (r *memoryWorkerRepository) MarkSucceeded(_ context.Context, id uint64, resultJSON string, reportID *uint, degraded bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.task.ID != id || r.task.Status != model.AnalysisTaskStatusRunning {
		return repository.ErrTaskStateChanged
	}
	now := time.Now()
	r.task.Status, r.task.ResultJSON, r.task.ReportID = model.AnalysisTaskStatusSucceeded, resultJSON, reportID
	r.task.Degraded, r.task.FinishedAt = degraded, &now
	r.task.ErrorCode, r.task.ErrorMessage = "", ""
	return nil
}

func (r *memoryWorkerRepository) MarkFailed(_ context.Context, id uint64, code, message string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.task.ID != id || r.task.Status != model.AnalysisTaskStatusRunning {
		return repository.ErrTaskStateChanged
	}
	now := time.Now()
	r.task.Status, r.task.ErrorCode, r.task.ErrorMessage = model.AnalysisTaskStatusFailed, code, message
	r.task.FinishedAt = &now
	return nil
}

func (r *memoryWorkerRepository) RecoverStaleTasks(context.Context, time.Time) (repository.StaleRecoveryResult, error) {
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
		Status: model.AnalysisTaskStatusRunning, AttemptCount: 1, MaxAttempts: 3,
	}
	return &memoryWorkerRepository{task: task}, task
}

func testWorkerConfig() config.AnalysisWorkerConfig {
	return config.AnalysisWorkerConfig{
		Enabled: true, WorkerCount: 1, PollIntervalMS: 10,
		TaskTimeoutSeconds: 1, StaleAfterSeconds: 60, MaxAttempts: 3,
	}
}
