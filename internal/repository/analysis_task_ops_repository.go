package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"pr-guard-agent/internal/model"
)

// TaskListFilter 定义了用于过滤分析任务列表的结构体，包含状态、项目ID、diff ID、错误码、降级标志、创建时间范围以及分页信息。
type TaskListFilter struct {
	Status      *string
	ProjectID   *uint
	DiffID      *uint
	ErrorCode   *string
	Degraded    *bool
	CreatedFrom *time.Time
	CreatedTo   *time.Time
	Page        int
	PageSize    int
}

// TaskMetrics 定义了分析任务的各种指标，包括待处理任务数、过期待处理任务数、计划重试任务数、运行中任务数、过期运行中任务数、最老待处理任务的年龄（秒）、提交任务数、成功任务数、失败任务数、未完成任务数、降级成功任务数、重试任务数、平均排队等待时间（毫秒）、平均运行时长（毫秒）以及最大运行时长（毫秒）。
type TaskMetrics struct {
	PendingCount            int64
	DuePendingCount         int64
	ScheduledRetryCount     int64
	RunningCount            int64
	StaleRunningCount       int64
	OldestPendingAgeSeconds int64

	SubmittedCount         int64
	SucceededCount         int64
	FailedCount            int64
	UnfinishedCount        int64
	DegradedSucceededCount int64
	RetriedTaskCount       int64
	AvgQueueWaitMS         float64
	AvgRunDurationMS       float64
	MaxRunDurationMS       float64
}

// ErrorCodeMetric 定义了错误码及其对应的任务数量，用于统计分析任务中不同错误码的出现频率。
type ErrorCodeMetric struct {
	ErrorCode string `json:"error_code"`
	Count     int64  `json:"count"`
}

// 应用过滤器，单独count，再仅选择安全字段分页查询；不加载关联对象，避免N+1查询问题。
func (r *AnalysisTaskRepository) ListTasks(ctx context.Context, filter TaskListFilter) ([]model.AnalysisTask, int64, error) {
	// 应用过滤器，单独进行计数查询，以获取满足条件的任务总数。
	query := applyTaskListFilter(r.db.WithContext(ctx).Model(&model.AnalysisTask{}), filter)

	var total int64
	// 执行计数查询，获取满足条件的任务总数。
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count analysis tasks failed: %w", err)
	}

	var tasks []model.AnalysisTask // 定义一个切片用于存储查询结果的分析任务。
	// 执行分页查询，仅选择安全字段，按创建时间和ID降序排序，避免加载关联对象以防止N+1查询问题。
	err := query.
		Select(
			"id",
			"project_id",
			"diff_id",
			"top_k",
			"status",
			"attempt_count",
			"max_attempts",
			"worker_id",
			"report_id",
			"degraded",
			"error_code",
			"error_message",
			"next_run_at",
			"started_at",
			"finished_at",
			"created_at",
			"update_at",
		).
		Order("created_at DESC").
		Order("id DESC").
		Offset((filter.Page - 1) * filter.PageSize).
		Limit(filter.PageSize).
		Find(&tasks).Error
	if err != nil {
		return nil, 0, fmt.Errorf("list analysis tasks failed: %w", err)
	}
	return tasks, total, nil
}

// 分别查询当前队列快照和时间窗口聚合，计算pending、due、scheduled、stale、成功/失败数及平均耗时。
func (r *AnalysisTaskRepository) GetTaskMetrics(
	ctx context.Context,
	since time.Time,
	staleBefore time.Time,
) (*TaskMetrics, error) {
	currentMetrics := &TaskMetrics{}
	currentQuery := `
SELECT
	COALESCE(SUM(status = ?), 0) AS pending_count,
	COALESCE(SUM(status = ? AND (next_run_at IS NULL OR next_run_at <= NOW())), 0) AS due_pending_count,
	COALESCE(SUM(status = ? AND attempt_count > 0 AND next_run_at > NOW()), 0) AS scheduled_retry_count,
	COALESCE(SUM(status = ?), 0) AS running_count,
	COALESCE(SUM(status = ? AND started_at < ?), 0) AS stale_running_count,
	COALESCE(GREATEST(TIMESTAMPDIFF(SECOND, MIN(CASE WHEN status = ? THEN created_at END), NOW()), 0), 0) AS oldest_pending_age_seconds
FROM analysis_tasks`
	if err := r.db.WithContext(ctx).Raw(
		currentQuery,
		model.AnalysisTaskStatusPending,
		model.AnalysisTaskStatusPending,
		model.AnalysisTaskStatusPending,
		model.AnalysisTaskStatusRunning,
		model.AnalysisTaskStatusRunning,
		staleBefore,
		model.AnalysisTaskStatusPending,
	).Scan(currentMetrics).Error; err != nil {
		return nil, fmt.Errorf("query current analysis task metrics failed: %w", err)
	}

	windowMetrics := &TaskMetrics{}
	windowQuery := `
SELECT
	COUNT(*) AS submitted_count,
	COALESCE(SUM(status = ?), 0) AS succeeded_count,
	COALESCE(SUM(status = ?), 0) AS failed_count,
	COALESCE(SUM(status IN (?, ?)), 0) AS unfinished_count,
	COALESCE(SUM(status = ? AND degraded = TRUE), 0) AS degraded_succeeded_count,
	COALESCE(SUM(attempt_count > 1), 0) AS retried_task_count,
	COALESCE(AVG(CASE
		WHEN started_at IS NOT NULL
		THEN TIMESTAMPDIFF(MICROSECOND, created_at, started_at) / 1000.0
	END), 0) AS avg_queue_wait_ms,
	COALESCE(AVG(CASE
		WHEN status IN (?, ?) AND started_at IS NOT NULL AND finished_at IS NOT NULL
		THEN TIMESTAMPDIFF(MICROSECOND, started_at, finished_at) / 1000.0
	END), 0) AS avg_run_duration_ms,
	COALESCE(MAX(CASE
		WHEN status IN (?, ?) AND started_at IS NOT NULL AND finished_at IS NOT NULL
		THEN TIMESTAMPDIFF(MICROSECOND, started_at, finished_at) / 1000.0
	END), 0) AS max_run_duration_ms
FROM analysis_tasks
WHERE created_at >= ?`
	if err := r.db.WithContext(ctx).Raw(
		windowQuery,
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
	).Scan(windowMetrics).Error; err != nil {
		return nil, fmt.Errorf("query window analysis task metrics failed: %w", err)
	}
	currentMetrics.SubmittedCount = windowMetrics.SubmittedCount
	currentMetrics.SucceededCount = windowMetrics.SucceededCount
	currentMetrics.FailedCount = windowMetrics.FailedCount
	currentMetrics.UnfinishedCount = windowMetrics.UnfinishedCount
	currentMetrics.DegradedSucceededCount = windowMetrics.DegradedSucceededCount
	currentMetrics.RetriedTaskCount = windowMetrics.RetriedTaskCount
	currentMetrics.AvgQueueWaitMS = windowMetrics.AvgQueueWaitMS
	currentMetrics.AvgRunDurationMS = windowMetrics.AvgRunDurationMS
	currentMetrics.MaxRunDurationMS = windowMetrics.MaxRunDurationMS
	return currentMetrics, nil
}

// 按失败错误码分组，空错误归为unknown，按数量降序，错误码升序，返回前limit个。
func (r *AnalysisTaskRepository) GetErrorsByCode(
	ctx context.Context,
	since time.Time,
	limit int,
) ([]ErrorCodeMetric, error) {
	var metrics []ErrorCodeMetric
	err := r.db.WithContext(ctx).
		Table("analysis_tasks").
		Select("COALESCE(NULLIF(error_code, ''), 'unknown') AS error_code, COUNT(*) AS count").
		Where("created_at >= ? AND status = ?", since, model.AnalysisTaskStatusFailed).
		Group("COALESCE(NULLIF(error_code, ''), 'unknown')").
		Order("count DESC").
		Order("error_code ASC").
		Limit(limit).
		Scan(&metrics).Error
	if err != nil {
		return nil, fmt.Errorf("query analysis task errors by code failed: %w", err)
	}
	return metrics, nil
}

// 按非nil条件逐个拼接GORM Where
func applyTaskListFilter(query *gorm.DB, filter TaskListFilter) *gorm.DB {
	if filter.Status != nil {
		query = query.Where("status = ?", *filter.Status)
	}
	if filter.ProjectID != nil {
		query = query.Where("project_id = ?", *filter.ProjectID)
	}
	if filter.DiffID != nil {
		query = query.Where("diff_id = ?", *filter.DiffID)
	}
	if filter.ErrorCode != nil {
		query = query.Where("error_code = ?", *filter.ErrorCode)
	}
	if filter.Degraded != nil {
		query = query.Where("degraded = ?", *filter.Degraded)
	}
	if filter.CreatedFrom != nil {
		query = query.Where("created_at >= ?", *filter.CreatedFrom)
	}
	if filter.CreatedTo != nil {
		query = query.Where("created_at <= ?", *filter.CreatedTo)
	}
	return query
}
