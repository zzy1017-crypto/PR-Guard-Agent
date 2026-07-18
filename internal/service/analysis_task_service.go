package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"pr-guard-agent/internal/model"
	"pr-guard-agent/internal/repository"
	"pr-guard-agent/internal/taskerror"
)

var (
	ErrInvalidAnalysisTopK        = taskerror.ErrInvalidTopK
	ErrAnalysisTaskRetryExhausted = errors.New("analysis task retry attempts exhausted")
	ErrAnalysisTaskInvalidStatus  = taskerror.ErrInvalidTaskState
)

type SubmitAnalysisTaskResult struct {
	Task    *model.AnalysisTask
	Reused  bool
	Retried bool
}

type analysisTaskRepository interface {
	Create(ctx context.Context, task *model.AnalysisTask) error
	GetByID(ctx context.Context, id uint64) (*model.AnalysisTask, error)
	GetByTaskKey(ctx context.Context, taskKey string) (*model.AnalysisTask, error)
	ResetFailedToPending(ctx context.Context, id uint64) error
}

type projectRepository interface {
	GetByIDWithContext(ctx context.Context, projectID uint) (*model.Project, error)
}

type diffRepository interface {
	GetByIDWithContext(ctx context.Context, diffID uint) (*model.DiffRecord, error)
}

type AnalysisTaskService struct {
	projectRepo projectRepository
	diffRepo    diffRepository
	taskRepo    analysisTaskRepository
	maxAttempts int
	logger      *zap.Logger
}

// 装配项目、diff和任务repository.
func NewAnalysisTaskService(db *gorm.DB, maxAttempts int, loggers ...*zap.Logger) *AnalysisTaskService {
	logger := zap.NewNop()
	if len(loggers) > 0 && loggers[0] != nil {
		logger = loggers[0]
	}
	return &AnalysisTaskService{
		projectRepo: repository.NewProjectRepository(db),
		diffRepo:    repository.NewDiffRepository(db),
		taskRepo:    repository.NewAnalysisTaskRepository(db),
		maxAttempts: maxAttempts,
		logger:      logger,
	}
}

// 对带长度前缀的项目/版本/diff/TopK串计算SHA-256哈希，作为分析任务的唯一Key。
func BuildAnalysisTaskKey(projectID uint, codeVersionHash string, diffHash string, topK int) string {
	payload := strconv.FormatUint(uint64(projectID), 10) + ":" +
		strconv.Itoa(len(codeVersionHash)) + ":" + codeVersionHash + ":" +
		strconv.Itoa(len(diffHash)) + ":" + diffHash + ":" +
		strconv.Itoa(topK)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

// 校验归属、生成任务键、复用已有任务或处理并发唯一键冲突，否则创建pending任务。
func (s *AnalysisTaskService) Submit(
	ctx context.Context,
	projectID uint,
	diffID uint,
	topK int,
	submitRequestID string,
) (*SubmitAnalysisTaskResult, error) {
	if topK < 1 || topK > 20 {
		return nil, ErrInvalidAnalysisTopK
	}

	project, err := s.projectRepo.GetByIDWithContext(ctx, projectID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("query project failed: %w", err)
	}
	diff, err := s.diffRepo.GetByIDWithContext(ctx, diffID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDiffNotFound
		}
		return nil, fmt.Errorf("query diff failed: %w", err)
	}
	if diff.ProjectID != project.ID {
		return nil, ErrDiffProjectMismatch
	}

	taskKey := BuildAnalysisTaskKey(project.ID, project.CodeVersionHash, diff.DiffHash, topK)
	existing, err := s.taskRepo.GetByTaskKey(ctx, taskKey)
	if err == nil {
		return s.resolveExisting(ctx, existing)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("query analysis task failed: %w", err)
	}

	task := &model.AnalysisTask{
		TaskKey:         taskKey,
		ProjectID:       project.ID,
		DiffID:          diff.ID,
		TopK:            topK,
		Status:          model.AnalysisTaskStatusPending,
		AttemptCount:    0,
		MaxAttempts:     s.maxAttempts,
		SubmitRequestID: submitRequestID,
	}
	if err := s.taskRepo.Create(ctx, task); err != nil {
		if repository.IsDuplicateKeyError(err) {
			existing, findErr := s.taskRepo.GetByTaskKey(ctx, taskKey)
			if findErr != nil {
				return nil, fmt.Errorf("query concurrently created analysis task failed: %w", findErr)
			}
			return s.resolveExisting(ctx, existing)
		}
		return nil, fmt.Errorf("create analysis task failed: %w", err)
	}

	s.logger.Info("analysis_task_created", taskLogFields(task)...)
	return &SubmitAnalysisTaskResult{Task: task}, nil
}

// 查询任务
func (s *AnalysisTaskService) Get(ctx context.Context, taskID uint64) (*model.AnalysisTask, error) {
	return s.taskRepo.GetByID(ctx, taskID)
}

// 复用未失败任务，未耗尽failed任务恢复pending，耗尽返回冲突。
func (s *AnalysisTaskService) resolveExisting(ctx context.Context, task *model.AnalysisTask) (*SubmitAnalysisTaskResult, error) {
	switch task.Status {
	case model.AnalysisTaskStatusPending, model.AnalysisTaskStatusRunning, model.AnalysisTaskStatusSucceeded:
		s.logger.Info("analysis_task_reused", taskLogFields(task)...)
		return &SubmitAnalysisTaskResult{Task: task, Reused: true}, nil
	case model.AnalysisTaskStatusFailed:
		if task.AttemptCount >= task.MaxAttempts {
			return nil, ErrAnalysisTaskRetryExhausted
		}
		if err := s.taskRepo.ResetFailedToPending(ctx, task.ID); err != nil {
			if errors.Is(err, repository.ErrTaskStateChanged) {
				latest, findErr := s.taskRepo.GetByTaskKey(ctx, task.TaskKey)
				if findErr != nil {
					return nil, fmt.Errorf("reload analysis task failed: %w", findErr)
				}
				return s.resolveExisting(ctx, latest)
			}
			return nil, fmt.Errorf("reset failed analysis task failed: %w", err)
		}
		latest, err := s.taskRepo.GetByID(ctx, task.ID)
		if err != nil {
			return nil, fmt.Errorf("reload retried analysis task failed: %w", err)
		}
		s.logger.Info("analysis_task_retried", taskLogFields(latest)...)
		return &SubmitAnalysisTaskResult{Task: latest, Retried: true}, nil
	default:
		return nil, ErrAnalysisTaskInvalidStatus
	}
}

// 生成统一的任务日志字段。
func taskLogFields(task *model.AnalysisTask) []zap.Field {
	return []zap.Field{
		zap.Uint64("task_id", task.ID),
		zap.Uint("project_id", task.ProjectID),
		zap.Uint("diff_id", task.DiffID),
	}
}
