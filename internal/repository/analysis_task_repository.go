package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"pr-guard-agent/internal/model"
)

var (
	ErrNoPendingTask    = errors.New("no pending analysis task")
	ErrTaskStateChanged = errors.New("analysis task state changed")
)

type StaleRecoveryResult struct {
	Pending int64
	Failed  int64
}

func (r StaleRecoveryResult) Total() int64 { return r.Pending + r.Failed }

type AnalysisTaskRepository struct {
	db *gorm.DB
}

func NewAnalysisTaskRepository(db *gorm.DB) *AnalysisTaskRepository {
	return &AnalysisTaskRepository{db: db}
}

func (r *AnalysisTaskRepository) Create(ctx context.Context, task *model.AnalysisTask) error {
	return r.db.WithContext(ctx).Create(task).Error
}

func (r *AnalysisTaskRepository) GetByID(ctx context.Context, id uint64) (*model.AnalysisTask, error) {
	var task model.AnalysisTask
	if err := r.db.WithContext(ctx).First(&task, id).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

func (r *AnalysisTaskRepository) GetByTaskKey(ctx context.Context, taskKey string) (*model.AnalysisTask, error) {
	var task model.AnalysisTask
	if err := r.db.WithContext(ctx).Where("task_key = ?", taskKey).First(&task).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

func IsDuplicateKeyError(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate entry") || strings.Contains(message, "unique constraint failed")
}

// ClaimNextPending locks one pending row and changes it to running in the same
// transaction. MySQL 8 uses SKIP LOCKED so concurrent workers do not wait on,
// or receive, the same row.
func (r *AnalysisTaskRepository) ClaimNextPending(ctx context.Context, workerID string) (*model.AnalysisTask, error) {
	var claimed *model.AnalysisTask
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var task model.AnalysisTask
		var result *gorm.DB
		if tx.Dialector.Name() == "mysql" {
			result = tx.Raw(`SELECT *
FROM analysis_tasks
WHERE status = ?
ORDER BY created_at, id
LIMIT 1
FOR UPDATE SKIP LOCKED`, model.AnalysisTaskStatusPending).Scan(&task)
		} else {
			// This branch keeps repository tests portable. Production is MySQL 8.
			result = tx.Where("status = ?", model.AnalysisTaskStatusPending).
				Order("created_at, id").Limit(1).Find(&task)
		}
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNoPendingTask
		}

		now := time.Now()
		update := tx.Model(&model.AnalysisTask{}).
			Where("id = ? AND status = ?", task.ID, model.AnalysisTaskStatusPending).
			Updates(map[string]any{
				"status":        model.AnalysisTaskStatusRunning,
				"attempt_count": gorm.Expr("attempt_count + 1"),
				"worker_id":     workerID,
				"started_at":    now,
				"finished_at":   nil,
				"update_at":     now,
			})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return ErrTaskStateChanged
		}

		task.Status = model.AnalysisTaskStatusRunning
		task.AttemptCount++
		task.WorkerID = workerID
		task.StartedAt = &now
		task.FinishedAt = nil
		task.UpdateAt = now
		claimed = &task
		return nil
	})
	if err != nil {
		return nil, err
	}
	return claimed, nil
}

func (r *AnalysisTaskRepository) MarkSucceeded(
	ctx context.Context,
	id uint64,
	resultJSON string,
	reportID *uint,
	degraded bool,
) error {
	now := time.Now()
	result := r.db.WithContext(ctx).Model(&model.AnalysisTask{}).
		Where("id = ? AND status = ?", id, model.AnalysisTaskStatusRunning).
		Updates(map[string]any{
			"status":        model.AnalysisTaskStatusSucceeded,
			"result_json":   resultJSON,
			"report_id":     reportID,
			"degraded":      degraded,
			"error_code":    "",
			"error_message": "",
			"finished_at":   now,
			"update_at":     now,
		})
	return stateUpdateError(result)
}

func (r *AnalysisTaskRepository) MarkFailed(ctx context.Context, id uint64, errorCode, errorMessage string) error {
	now := time.Now()
	result := r.db.WithContext(ctx).Model(&model.AnalysisTask{}).
		Where("id = ? AND status = ?", id, model.AnalysisTaskStatusRunning).
		Updates(map[string]any{
			"status":        model.AnalysisTaskStatusFailed,
			"error_code":    errorCode,
			"error_message": errorMessage,
			"finished_at":   now,
			"update_at":     now,
		})
	return stateUpdateError(result)
}

func (r *AnalysisTaskRepository) ResetFailedToPending(ctx context.Context, id uint64) error {
	now := time.Now()
	result := r.db.WithContext(ctx).Model(&model.AnalysisTask{}).
		Where("id = ? AND status = ? AND attempt_count < max_attempts", id, model.AnalysisTaskStatusFailed).
		Updates(map[string]any{
			"status":        model.AnalysisTaskStatusPending,
			"error_code":    "",
			"error_message": "",
			"worker_id":     "",
			"started_at":    nil,
			"finished_at":   nil,
			"update_at":     now,
		})
	return stateUpdateError(result)
}

func (r *AnalysisTaskRepository) RecoverStaleTasks(ctx context.Context, cutoff time.Time) (StaleRecoveryResult, error) {
	staleWhere := "status = ? AND (started_at < ? OR (started_at IS NULL AND update_at < ?))"
	now := time.Now()

	pending := r.db.WithContext(ctx).Model(&model.AnalysisTask{}).
		Where(staleWhere+" AND attempt_count < max_attempts", model.AnalysisTaskStatusRunning, cutoff, cutoff).
		Updates(map[string]any{
			"status":      model.AnalysisTaskStatusPending,
			"worker_id":   "",
			"started_at":  nil,
			"finished_at": nil,
			"update_at":   now,
		})
	if pending.Error != nil {
		return StaleRecoveryResult{}, pending.Error
	}

	failed := r.db.WithContext(ctx).Model(&model.AnalysisTask{}).
		Where(staleWhere+" AND attempt_count >= max_attempts", model.AnalysisTaskStatusRunning, cutoff, cutoff).
		Updates(map[string]any{
			"status":        model.AnalysisTaskStatusFailed,
			"worker_id":     "",
			"error_code":    "task_stale_retry_exhausted",
			"error_message": "analysis task retry attempts exhausted after stale execution",
			"finished_at":   now,
			"update_at":     now,
		})
	if failed.Error != nil {
		return StaleRecoveryResult{}, failed.Error
	}

	return StaleRecoveryResult{Pending: pending.RowsAffected, Failed: failed.RowsAffected}, nil
}

func stateUpdateError(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrTaskStateChanged
	}
	return nil
}
