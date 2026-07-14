package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"pr-guard-agent/internal/model"
	"pr-guard-agent/internal/taskerror"
)

var (
	ErrNoPendingTask     = errors.New("no pending analysis task")
	ErrTaskStateConflict = taskerror.ErrInvalidTaskState
	ErrTaskStateChanged  = ErrTaskStateConflict
)

type StaleRecoveryItem struct {
	TaskID       uint64
	WorkerID     string
	AttemptCount int
	MaxAttempts  int
	NextRunAt    *time.Time
}

type StaleRecoveryResult struct {
	Pending   int64
	Failed    int64
	Scheduled []StaleRecoveryItem
	Exhausted []StaleRecoveryItem
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
		now := time.Now()
		var task model.AnalysisTask
		var result *gorm.DB
		if tx.Dialector.Name() == "mysql" {
			result = tx.Raw(`SELECT *
FROM analysis_tasks
WHERE status = ?
AND (next_run_at IS NULL OR next_run_at <= ?)
ORDER BY created_at, id
LIMIT 1
FOR UPDATE SKIP LOCKED`, model.AnalysisTaskStatusPending, now).Scan(&task)
		} else {
			// This branch keeps repository tests portable. Production is MySQL 8.
			result = tx.Where("status = ? AND (next_run_at IS NULL OR next_run_at <= ?)", model.AnalysisTaskStatusPending, now).
				Order("created_at, id").Limit(1).Find(&task)
		}
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNoPendingTask
		}

		update := tx.Model(&model.AnalysisTask{}).
			Where("id = ? AND status = ?", task.ID, model.AnalysisTaskStatusPending).
			Updates(map[string]any{
				"status":        model.AnalysisTaskStatusRunning,
				"attempt_count": gorm.Expr("attempt_count + 1"),
				"worker_id":     workerID,
				"started_at":    now,
				"next_run_at":   nil,
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
		task.NextRunAt = nil
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
	workerID string,
	expectedAttempt int,
	resultJSON string,
	reportID *uint,
	degraded bool,
) error {
	now := time.Now()
	result := r.db.WithContext(ctx).Model(&model.AnalysisTask{}).
		Where("id = ? AND status = ? AND worker_id = ? AND attempt_count = ?", id, model.AnalysisTaskStatusRunning, workerID, expectedAttempt).
		Updates(map[string]any{
			"status":        model.AnalysisTaskStatusSucceeded,
			"result_json":   resultJSON,
			"report_id":     reportID,
			"degraded":      degraded,
			"error_code":    "",
			"error_message": "",
			"next_run_at":   nil,
			"finished_at":   now,
			"update_at":     now,
		})
	return stateUpdateError(result)
}

func (r *AnalysisTaskRepository) RequeueWithBackoff(
	ctx context.Context,
	taskID uint64,
	workerID string,
	expectedAttempt int,
	nextRunAt time.Time,
	errorCode string,
	errorMessage string,
) error {
	now := time.Now()
	result := r.db.WithContext(ctx).Model(&model.AnalysisTask{}).
		Where("id = ? AND status = ? AND worker_id = ? AND attempt_count = ?", taskID, model.AnalysisTaskStatusRunning, workerID, expectedAttempt).
		Updates(map[string]any{
			"status":         model.AnalysisTaskStatusPending,
			"worker_id":      "",
			"started_at":     nil,
			"finished_at":    nil,
			"next_run_at":    nextRunAt,
			"error_code":     errorCode,
			"error_message":  errorMessage,
			"last_failed_at": now,
			"update_at":      now,
		})
	return stateUpdateError(result)
}

func (r *AnalysisTaskRepository) MarkFailedFinal(
	ctx context.Context,
	taskID uint64,
	workerID string,
	expectedAttempt int,
	errorCode string,
	errorMessage string,
) error {
	now := time.Now()
	result := r.db.WithContext(ctx).Model(&model.AnalysisTask{}).
		Where("id = ? AND status = ? AND worker_id = ? AND attempt_count = ?", taskID, model.AnalysisTaskStatusRunning, workerID, expectedAttempt).
		Updates(map[string]any{
			"status":         model.AnalysisTaskStatusFailed,
			"next_run_at":    nil,
			"error_code":     errorCode,
			"error_message":  errorMessage,
			"last_failed_at": now,
			"finished_at":    now,
			"update_at":      now,
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
			"next_run_at":   nil,
			"update_at":     now,
		})
	return stateUpdateError(result)
}

func (r *AnalysisTaskRepository) RecoverStaleTasks(
	ctx context.Context,
	cutoff time.Time,
	policy taskerror.RetryPolicy,
) (StaleRecoveryResult, error) {
	var recovered StaleRecoveryResult
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tasks []model.AnalysisTask
		var query *gorm.DB
		if tx.Dialector.Name() == "mysql" {
			query = tx.Raw(`SELECT id, worker_id, attempt_count, max_attempts
FROM analysis_tasks
WHERE status = ?
AND started_at < ?
FOR UPDATE SKIP LOCKED`, model.AnalysisTaskStatusRunning, cutoff).Scan(&tasks)
		} else {
			query = tx.Where("status = ? AND started_at < ?", model.AnalysisTaskStatusRunning, cutoff).Find(&tasks)
		}
		if query.Error != nil {
			return query.Error
		}

		now := time.Now()
		for _, task := range tasks {
			where := tx.Model(&model.AnalysisTask{}).
				Where("id = ? AND status = ? AND worker_id = ? AND attempt_count = ? AND started_at < ?", task.ID, model.AnalysisTaskStatusRunning, task.WorkerID, task.AttemptCount, cutoff)
			item := StaleRecoveryItem{TaskID: task.ID, WorkerID: task.WorkerID, AttemptCount: task.AttemptCount, MaxAttempts: task.MaxAttempts}
			if task.AttemptCount < task.MaxAttempts {
				nextRunAt := now.Add(policy.NextDelay(task.AttemptCount))
				update := where.Updates(map[string]any{
					"status":         model.AnalysisTaskStatusPending,
					"worker_id":      "",
					"started_at":     nil,
					"finished_at":    nil,
					"next_run_at":    nextRunAt,
					"error_code":     taskerror.CodeWorkerStale,
					"error_message":  "analysis worker execution became stale",
					"last_failed_at": now,
					"update_at":      now,
				})
				if err := stateUpdateError(update); err != nil {
					return err
				}
				item.NextRunAt = &nextRunAt
				recovered.Pending++
				recovered.Scheduled = append(recovered.Scheduled, item)
				continue
			}

			update := where.Updates(map[string]any{
				"status":         model.AnalysisTaskStatusFailed,
				"next_run_at":    nil,
				"error_code":     taskerror.CodeRetryExhausted,
				"error_message":  "analysis failed after maximum attempts: worker execution became stale",
				"last_failed_at": now,
				"finished_at":    now,
				"update_at":      now,
			})
			if err := stateUpdateError(update); err != nil {
				return err
			}
			recovered.Failed++
			recovered.Exhausted = append(recovered.Exhausted, item)
		}
		return nil
	})
	if err != nil {
		return StaleRecoveryResult{}, err
	}
	return recovered, nil
}

func stateUpdateError(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrTaskStateConflict
	}
	return nil
}
