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

// StaleRecoveryItem 表示一个过期任务的恢复项，包含任务ID、工作节点ID、尝试次数、最大尝试次数和下一次运行时间。
type StaleRecoveryItem struct {
	TaskID       uint64
	WorkerID     string
	AttemptCount int
	MaxAttempts  int
	NextRunAt    *time.Time
}

// StaleRecoveryResult 表示过期任务恢复的结果，包含恢复的挂起任务数、失败任务数、计划恢复的任务列表和耗尽尝试的任务列表。
type StaleRecoveryResult struct {
	Pending   int64
	Failed    int64
	Scheduled []StaleRecoveryItem
	Exhausted []StaleRecoveryItem
}

// Total 返回恢复的总任务数，即重新排队数加上失败任务数。
func (r StaleRecoveryResult) Total() int64 { return r.Pending + r.Failed }

// AnalysisTaskRepository 提供对分析任务的数据库操作接口，封装了对分析任务的增删改查和状态管理等功能。
type AnalysisTaskRepository struct {
	db *gorm.DB
}

// NewAnalysisTaskRepository 创建一个新的 AnalysisTaskRepository 实例，接受一个 gorm.DB 对象作为参数，用于数据库操作。
func NewAnalysisTaskRepository(db *gorm.DB) *AnalysisTaskRepository {
	return &AnalysisTaskRepository{db: db}
}

// 用于请求Context创建任务
func (r *AnalysisTaskRepository) Create(ctx context.Context, task *model.AnalysisTask) error {
	return r.db.WithContext(ctx).Create(task).Error
}

// GetByID 根据任务ID获取分析任务，如果任务不存在则返回gorm.Err
func (r *AnalysisTaskRepository) GetByID(ctx context.Context, id uint64) (*model.AnalysisTask, error) {
	var task model.AnalysisTask
	if err := r.db.WithContext(ctx).First(&task, id).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

// GetByTaskKey 根据任务键获取分析任务，如果任务不存在则返回gorm.Err
func (r *AnalysisTaskRepository) GetByTaskKey(ctx context.Context, taskKey string) (*model.AnalysisTask, error) {
	var task model.AnalysisTask
	if err := r.db.WithContext(ctx).Where("task_key = ?", taskKey).First(&task).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

// 兼容GORM、MySQL 1062和常见错误文本识别唯一键冲突；用于处理并发幂等提交。
func IsDuplicateKeyError(err error) bool {

	//如果错误为ErrDuplicatedKey，返回true。
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}

	var mysqlErr *mysqlDriver.MySQLError //定义一个MySQL错误类型的变量，用于检查是否为MySQL特定的错误。

	// 检查错误是否为MySQL的唯一键冲突错误（错误码1062），如果是则返回true。
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return true
	}
	// 将错误信息转换为小写字符串，并检查是否包含"duplicate entry"或"unique constraint failed"等关键字，以判断是否为唯一键冲突错误。
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate entry") || strings.Contains(message, "unique constraint failed")
}

// 事务中选择到期pending任务，MySQL使用FOR UPDATE SKIP LOCKED,随后条件更新为running并递增次数。
func (r *AnalysisTaskRepository) ClaimNextPending(ctx context.Context, workerID string) (*model.AnalysisTask, error) {
	//定义一个指向model.AnalysisTask的指针变量claimed，用于存储被领取的任务。
	var claimed *model.AnalysisTask
	// 在数据库事务中执行领取任务的操作，确保操作的原子性和一致性。
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()           //获取当前时间，用于比较任务的下一次运行时间和记录任务的开始时间。
		var task model.AnalysisTask //定义一个model.AnalysisTask类型的变量task，用于存储查询到的待领取任务。
		var result *gorm.DB         //定义一个指向gorm.DB的指针变量result，用于存储数据库查询和更新的结果。
		// 根据数据库类型选择不同的查询方式，MySQL使用FOR UPDATE SKIP LOCKED以避免锁冲突，其他数据库使用普通查询。
		if tx.Dialector.Name() == "mysql" {
			result = tx.Raw(`SELECT *
FROM analysis_tasks
WHERE status = ?
AND (next_run_at IS NULL OR next_run_at <= ?)
ORDER BY created_at, id
LIMIT 1
FOR UPDATE SKIP LOCKED`, model.AnalysisTaskStatusPending, now).Scan(&task)
		} else {
			// 对于非MySQL数据库，使用普通的查询方式，按创建时间和ID升序排序，限制返回一条记录。
			result = tx.Where("status = ? AND (next_run_at IS NULL OR next_run_at <= ?)", model.AnalysisTaskStatusPending, now).
				Order("created_at, id").Limit(1).Find(&task)
		}
		// 检查查询结果，如果发生错误则返回错误；如果没有查询到任务，则返回ErrNoPendingTask。
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNoPendingTask
		}

		// 在事务中更新任务的状态为running，并递增尝试次数，同时记录工作节点ID和开始时间等信息。
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
		// 检查更新结果，如果发生错误则返回错误；如果更新的行数不为1，则说明任务状态已被其他事务修改，返回ErrTaskStateChanged。
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return ErrTaskStateChanged
		}

		// 将领取的任务赋值给claimed变量，以便在事务外部返回。
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
	// 如果事务执行过程中发生错误，则返回错误；否则返回领取的任务。
	if err != nil {
		return nil, err
	}
	return claimed, nil
}

// MarkSucceeded 将指定任务标记为成功完成，更新任务的状态、结果JSON、报告ID、降级标志等信息，并记录完成时间。
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
	// 在数据库中更新任务的状态为succeeded，并设置相关字段的值，同时检查更新结果是否成功。
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
	// 调用stateUpdateError函数检查更新结果。
	return stateUpdateError(result)
}

// 用相同乐观状态条件把任务回复pending，并写下一次执行时间和安全错误。
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

// 将条件更新为failed,记录最终错误和结束时间。
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

// 仅在attempt未耗尽时把failed任务恢复。
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

// 锁定过期running任务；未耗尽则计算退避并重排，耗尽则最终失败。
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

// 数据库错误原样返回，受影响行不是1时返回状态冲突；防止静默覆盖
func stateUpdateError(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrTaskStateConflict
	}
	return nil
}
