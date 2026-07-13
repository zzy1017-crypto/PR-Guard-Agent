package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"pr-guard-agent/internal/model"
	"pr-guard-agent/internal/repository"
)

func TestBuildAnalysisTaskKeyStableAndTopKSensitive(t *testing.T) {
	first := BuildAnalysisTaskKey(7, "code-version", "diff-hash", 5)
	second := BuildAnalysisTaskKey(7, "code-version", "diff-hash", 5)
	if first != second {
		t.Fatalf("same input returned different keys: %q != %q", first, second)
	}
	if len(first) != 64 {
		t.Fatalf("key length = %d, want 64", len(first))
	}
	if first == BuildAnalysisTaskKey(7, "code-version", "diff-hash", 6) {
		t.Fatal("different top_k returned the same key")
	}
}

func TestSubmitAnalysisTaskCreatesPendingAndReusesIt(t *testing.T) {
	svc, _, _ := inMemoryAnalysisTaskService()
	created, err := svc.Submit(context.Background(), 1, 2, 5, "request-1")
	if err != nil {
		t.Fatalf("Submit(create) error = %v", err)
	}
	if created.Reused || created.Retried || created.Task.Status != model.AnalysisTaskStatusPending {
		t.Fatalf("unexpected created result: %#v", created)
	}
	if created.Task.AttemptCount != 0 || created.Task.MaxAttempts != 3 || created.Task.SubmitRequestID != "request-1" {
		t.Fatalf("unexpected created task: %#v", created.Task)
	}

	reused, err := svc.Submit(context.Background(), 1, 2, 5, "request-2")
	if err != nil {
		t.Fatalf("Submit(reuse) error = %v", err)
	}
	if !reused.Reused || reused.Retried || reused.Task.ID != created.Task.ID {
		t.Fatalf("unexpected reused result: %#v", reused)
	}
}

func TestSubmitAnalysisTaskHandlesDuplicateCreateByReloading(t *testing.T) {
	svc, project, diff := inMemoryAnalysisTaskService()
	existing := &model.AnalysisTask{
		ID:          99,
		TaskKey:     BuildAnalysisTaskKey(project.ID, project.CodeVersionHash, diff.DiffHash, 5),
		ProjectID:   project.ID,
		DiffID:      diff.ID,
		TopK:        5,
		Status:      model.AnalysisTaskStatusPending,
		MaxAttempts: 3,
	}
	svc.taskRepo = &duplicateOnCreateTaskRepository{existing: existing}

	result, err := svc.Submit(context.Background(), project.ID, diff.ID, 5, "request-race")
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if !result.Reused || result.Task.ID != existing.ID {
		t.Fatalf("duplicate create was not reloaded: %#v", result)
	}
}

func TestSubmitAnalysisTaskRetriesFailedUntilMaxAttempts(t *testing.T) {
	svc, _, _ := inMemoryAnalysisTaskService()
	created, err := svc.Submit(context.Background(), 1, 2, 5, "request-1")
	if err != nil {
		t.Fatal(err)
	}
	repo := svc.taskRepo.(*memoryAnalysisTaskRepository)
	created.Task.Status = model.AnalysisTaskStatusFailed
	created.Task.AttemptCount = 2
	created.Task.ErrorCode = "analysis_failed"
	created.Task.ErrorMessage = "safe"

	retried, err := svc.Submit(context.Background(), 1, 2, 5, "request-2")
	if err != nil {
		t.Fatalf("Submit(retry) error = %v", err)
	}
	if !retried.Retried || retried.Reused || retried.Task.Status != model.AnalysisTaskStatusPending || retried.Task.AttemptCount != 2 {
		t.Fatalf("unexpected retry result: %#v", retried)
	}

	repo.tasks[created.Task.TaskKey].Status = model.AnalysisTaskStatusFailed
	repo.tasks[created.Task.TaskKey].AttemptCount = 3
	_, err = svc.Submit(context.Background(), 1, 2, 5, "request-3")
	if !errors.Is(err, ErrAnalysisTaskRetryExhausted) {
		t.Fatalf("Submit(exhausted) error = %v, want %v", err, ErrAnalysisTaskRetryExhausted)
	}
}

func inMemoryAnalysisTaskService() (*AnalysisTaskService, *model.Project, *model.DiffRecord) {
	project := &model.Project{ID: 1, Name: "project", CodeVersionHash: "code-version"}
	diff := &model.DiffRecord{ID: 2, ProjectID: 1, DiffHash: "diff-hash"}
	return &AnalysisTaskService{
		projectRepo: &fixedProjectRepository{project: project},
		diffRepo:    &fixedDiffRepository{diff: diff},
		taskRepo:    &memoryAnalysisTaskRepository{tasks: make(map[string]*model.AnalysisTask)},
		maxAttempts: 3,
		logger:      zap.NewNop(),
	}, project, diff
}

type fixedProjectRepository struct{ project *model.Project }

func (r *fixedProjectRepository) GetByIDWithContext(context.Context, uint) (*model.Project, error) {
	return r.project, nil
}

type fixedDiffRepository struct{ diff *model.DiffRecord }

func (r *fixedDiffRepository) GetByIDWithContext(context.Context, uint) (*model.DiffRecord, error) {
	return r.diff, nil
}

type memoryAnalysisTaskRepository struct {
	mu     sync.Mutex
	nextID uint64
	tasks  map[string]*model.AnalysisTask
}

func (r *memoryAnalysisTaskRepository) Create(_ context.Context, task *model.AnalysisTask) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tasks[task.TaskKey]; exists {
		return &mysqlDriver.MySQLError{Number: 1062, Message: "Duplicate entry"}
	}
	r.nextID++
	task.ID = r.nextID
	task.CreatedAt = time.Now()
	r.tasks[task.TaskKey] = task
	return nil
}

func (r *memoryAnalysisTaskRepository) GetByID(_ context.Context, id uint64) (*model.AnalysisTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, task := range r.tasks {
		if task.ID == id {
			return task, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *memoryAnalysisTaskRepository) GetByTaskKey(_ context.Context, key string) (*model.AnalysisTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[key]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return task, nil
}

func (r *memoryAnalysisTaskRepository) ResetFailedToPending(_ context.Context, id uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, task := range r.tasks {
		if task.ID == id && task.Status == model.AnalysisTaskStatusFailed && task.AttemptCount < task.MaxAttempts {
			task.Status = model.AnalysisTaskStatusPending
			task.ErrorCode, task.ErrorMessage, task.WorkerID = "", "", ""
			task.StartedAt, task.FinishedAt = nil, nil
			return nil
		}
	}
	return repository.ErrTaskStateChanged
}

type duplicateOnCreateTaskRepository struct {
	existing *model.AnalysisTask
	lookups  int
}

func (r *duplicateOnCreateTaskRepository) Create(context.Context, *model.AnalysisTask) error {
	return &mysqlDriver.MySQLError{Number: 1062, Message: "Duplicate entry"}
}

func (r *duplicateOnCreateTaskRepository) GetByID(context.Context, uint64) (*model.AnalysisTask, error) {
	return r.existing, nil
}

func (r *duplicateOnCreateTaskRepository) GetByTaskKey(context.Context, string) (*model.AnalysisTask, error) {
	r.lookups++
	if r.lookups == 1 {
		return nil, gorm.ErrRecordNotFound
	}
	return r.existing, nil
}

func (r *duplicateOnCreateTaskRepository) ResetFailedToPending(context.Context, uint64) error {
	return nil
}
