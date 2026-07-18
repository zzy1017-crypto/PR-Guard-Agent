package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"pr-guard-agent/internal/config"
	"pr-guard-agent/internal/model"
	"pr-guard-agent/internal/repository"
	"pr-guard-agent/internal/runtimeinfo"
)

const opsErrorCodeLimit = 20

type analysisTaskOpsRepository interface {
	ListTasks(ctx context.Context, filter repository.TaskListFilter) ([]model.AnalysisTask, int64, error)
	GetTaskMetrics(ctx context.Context, since time.Time, staleBefore time.Time) (*repository.TaskMetrics, error)
	GetErrorsByCode(ctx context.Context, since time.Time, limit int) ([]repository.ErrorCodeMetric, error)
}

type workerRuntimeSnapshotProvider interface {
	Snapshot() runtimeinfo.WorkerRuntimeSnapshot
}

type AnalysisTaskOpsService struct {
	repository     analysisTaskOpsRepository
	opsConfig      config.OpsConfig
	workerConfig   config.AnalysisWorkerConfig
	workerRegistry workerRuntimeSnapshotProvider
	workerStopping func() bool
	now            func() time.Time
}

type TaskListResult struct {
	Total    int64          `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
	Items    []TaskListItem `json:"items"`
}

type TaskListItem struct {
	TaskID         uint64     `json:"task_id"`
	ProjectID      uint       `json:"project_id"`
	DiffID         uint       `json:"diff_id"`
	TopK           int        `json:"top_k"`
	Status         string     `json:"status"`
	AttemptCount   int        `json:"attempt_count"`
	MaxAttempts    int        `json:"max_attempts"`
	WorkerID       string     `json:"worker_id"`
	ReportID       *uint      `json:"report_id"`
	Degraded       bool       `json:"degraded"`
	ErrorCode      string     `json:"error_code"`
	ErrorMessage   string     `json:"error_message"`
	RetryScheduled bool       `json:"retry_scheduled"`
	NextRunAt      *time.Time `json:"next_run_at"`
	StartedAt      *time.Time `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdateAt       time.Time  `json:"update_at"`
}

type TaskMetricsResult struct {
	WindowHours  int                          `json:"window_hours"`
	Since        time.Time                    `json:"since"`
	Current      CurrentTaskMetrics           `json:"current"`
	Window       WindowTaskMetrics            `json:"window"`
	ErrorsByCode []repository.ErrorCodeMetric `json:"errors_by_code"`
}

type CurrentTaskMetrics struct {
	PendingCount            int64 `json:"pending_count"`
	DuePendingCount         int64 `json:"due_pending_count"`
	ScheduledRetryCount     int64 `json:"scheduled_retry_count"`
	RunningCount            int64 `json:"running_count"`
	StaleRunningCount       int64 `json:"stale_running_count"`
	OldestPendingAgeSeconds int64 `json:"oldest_pending_age_seconds"`
}

type WindowTaskMetrics struct {
	SubmittedCount         int64   `json:"submitted_count"`
	SucceededCount         int64   `json:"succeeded_count"`
	FailedCount            int64   `json:"failed_count"`
	UnfinishedCount        int64   `json:"unfinished_count"`
	DegradedSucceededCount int64   `json:"degraded_succeeded_count"`
	RetriedTaskCount       int64   `json:"retried_task_count"`
	SuccessRatePercent     float64 `json:"success_rate_percent"`
	FailureRatePercent     float64 `json:"failure_rate_percent"`
	DegradedRatePercent    float64 `json:"degraded_rate_percent"`
	RetryRatePercent       float64 `json:"retry_rate_percent"`
	AvgQueueWaitMS         float64 `json:"avg_queue_wait_ms"`
	AvgRunDurationMS       float64 `json:"avg_run_duration_ms"`
	MaxRunDurationMS       float64 `json:"max_run_duration_ms"`
}

type WorkerStatusResult struct {
	ManagerEnabled               bool               `json:"manager_enabled"`
	ConfiguredWorkerCount        int                `json:"configured_worker_count"`
	RegisteredWorkerCount        int                `json:"registered_worker_count"`
	BusyWorkerCount              int                `json:"busy_worker_count"`
	IdleWorkerCount              int                `json:"idle_worker_count"`
	RuntimeStartedAt             time.Time          `json:"runtime_started_at"`
	UptimeSeconds                int64              `json:"uptime_seconds"`
	RuntimeMetricsResetOnRestart bool               `json:"runtime_metrics_reset_on_restart"`
	Stopping                     bool               `json:"stopping"`
	Workers                      []WorkerStatusItem `json:"workers"`
}

type WorkerStatusItem struct {
	WorkerID       string     `json:"worker_id"`
	Busy           bool       `json:"busy"`
	CurrentTaskID  uint64     `json:"current_task_id"`
	LastPollAt     *time.Time `json:"last_poll_at"`
	LastClaimAt    *time.Time `json:"last_claim_at"`
	LastSuccessAt  *time.Time `json:"last_success_at"`
	LastFailureAt  *time.Time `json:"last_failure_at"`
	ProcessedCount uint64     `json:"processed_count"`
	SuccessCount   uint64     `json:"success_count"`
	FailureCount   uint64     `json:"failure_count"`
}

// 注入Repository、配置、运行快照和stopping回调。
func NewAnalysisTaskOpsService(
	repo analysisTaskOpsRepository,
	opsConfig config.OpsConfig,
	workerConfig config.AnalysisWorkerConfig,
	workerRegistry workerRuntimeSnapshotProvider,
	workerStopping func() bool,
) *AnalysisTaskOpsService {
	return &AnalysisTaskOpsService{
		repository:     repo,
		opsConfig:      opsConfig,
		workerConfig:   workerConfig,
		workerRegistry: workerRegistry,
		workerStopping: workerStopping,
		now:            time.Now,
	}
}

// ListTasks 查询分析任务列表，支持分页、过滤和排序，返回任务列表及总数。把数据库模型转换为安全DTO，清洗错误文本。
func (s *AnalysisTaskOpsService) ListTasks(ctx context.Context, filter repository.TaskListFilter) (*TaskListResult, error) {
	if s == nil || s.repository == nil {
		return nil, errors.New("analysis task ops repository is not initialized")
	}
	tasks, total, err := s.repository.ListTasks(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list analysis tasks for operations failed: %w", err)
	}
	items := make([]TaskListItem, 0, len(tasks))
	for _, task := range tasks {
		items = append(items, TaskListItem{
			TaskID:         task.ID,
			ProjectID:      task.ProjectID,
			DiffID:         task.DiffID,
			TopK:           task.TopK,
			Status:         task.Status,
			AttemptCount:   task.AttemptCount,
			MaxAttempts:    task.MaxAttempts,
			WorkerID:       task.WorkerID,
			ReportID:       task.ReportID,
			Degraded:       task.Degraded,
			ErrorCode:      task.ErrorCode,
			ErrorMessage:   safeOpsErrorMessage(task.ErrorMessage),
			RetryScheduled: task.Status == model.AnalysisTaskStatusPending && task.AttemptCount > 0 && task.NextRunAt != nil,
			NextRunAt:      task.NextRunAt,
			StartedAt:      task.StartedAt,
			FinishedAt:     task.FinishedAt,
			CreatedAt:      task.CreatedAt,
			UpdateAt:       task.UpdateAt,
		})
	}
	return &TaskListResult{
		Total:    total,
		Page:     filter.Page,
		PageSize: filter.PageSize,
		Items:    items,
	}, nil
}

// 计算时间边界、读取聚合数据并计算成功率、失败率、降级率和重试率。
func (s *AnalysisTaskOpsService) GetMetrics(ctx context.Context, windowHours int) (*TaskMetricsResult, error) {
	if s == nil || s.repository == nil {
		return nil, errors.New("analysis task ops repository is not initialized")
	}
	if windowHours <= 0 || windowHours > s.opsConfig.MaxMetricsWindowHours {
		return nil, errors.New("invalid operations metrics window")
	}
	now := s.now()
	since := now.Add(-time.Duration(windowHours) * time.Hour)
	staleBefore := now.Add(-time.Duration(s.workerConfig.StaleAfterSeconds) * time.Second)
	metrics, err := s.repository.GetTaskMetrics(ctx, since, staleBefore)
	if err != nil {
		return nil, fmt.Errorf("get analysis task metrics failed: %w", err)
	}
	errorsByCode, err := s.repository.GetErrorsByCode(ctx, since, opsErrorCodeLimit)
	if err != nil {
		return nil, fmt.Errorf("get analysis task error distribution failed: %w", err)
	}

	terminalCount := metrics.SucceededCount + metrics.FailedCount
	return &TaskMetricsResult{
		WindowHours: windowHours,
		Since:       since,
		Current: CurrentTaskMetrics{
			PendingCount:            metrics.PendingCount,
			DuePendingCount:         metrics.DuePendingCount,
			ScheduledRetryCount:     metrics.ScheduledRetryCount,
			RunningCount:            metrics.RunningCount,
			StaleRunningCount:       metrics.StaleRunningCount,
			OldestPendingAgeSeconds: metrics.OldestPendingAgeSeconds,
		},
		Window: WindowTaskMetrics{
			SubmittedCount:         metrics.SubmittedCount,
			SucceededCount:         metrics.SucceededCount,
			FailedCount:            metrics.FailedCount,
			UnfinishedCount:        metrics.UnfinishedCount,
			DegradedSucceededCount: metrics.DegradedSucceededCount,
			RetriedTaskCount:       metrics.RetriedTaskCount,
			SuccessRatePercent:     percentage(metrics.SucceededCount, terminalCount),
			FailureRatePercent:     percentage(metrics.FailedCount, terminalCount),
			DegradedRatePercent:    percentage(metrics.DegradedSucceededCount, metrics.SucceededCount),
			RetryRatePercent:       percentage(metrics.RetriedTaskCount, metrics.SubmittedCount),
			AvgQueueWaitMS:         metrics.AvgQueueWaitMS,
			AvgRunDurationMS:       metrics.AvgRunDurationMS,
			MaxRunDurationMS:       metrics.MaxRunDurationMS,
		},
		ErrorsByCode: errorsByCode,
	}, nil
}

// 查询分析任务工作器状态，返回注册的工作器数量、忙碌和空闲工作器数量、运行时间、运行时指标是否在重启时重置以及每个工作器的详细状态。
func (s *AnalysisTaskOpsService) GetWorkers(ctx context.Context) (*WorkerStatusResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.workerRegistry == nil {
		return nil, errors.New("worker runtime registry is not initialized")
	}
	now := s.now()
	snapshot := s.workerRegistry.Snapshot()
	workers := make([]WorkerStatusItem, 0, len(snapshot.Workers))
	busyCount := 0
	for _, worker := range snapshot.Workers {
		if worker.Busy {
			busyCount++
		}
		workers = append(workers, WorkerStatusItem{
			WorkerID:       worker.WorkerID,
			Busy:           worker.Busy,
			CurrentTaskID:  worker.CurrentTaskID,
			LastPollAt:     worker.LastPollAt,
			LastClaimAt:    worker.LastClaimAt,
			LastSuccessAt:  worker.LastSuccessAt,
			LastFailureAt:  worker.LastFailureAt,
			ProcessedCount: worker.ProcessedCount,
			SuccessCount:   worker.SuccessCount,
			FailureCount:   worker.FailureCount,
		})
	}
	uptime := int64(now.Sub(snapshot.StartedAt).Seconds())
	if snapshot.StartedAt.IsZero() || uptime < 0 {
		uptime = 0
	}
	stopping := false
	if s.workerStopping != nil {
		stopping = s.workerStopping()
	}
	return &WorkerStatusResult{
		ManagerEnabled:               s.workerConfig.Enabled,
		ConfiguredWorkerCount:        s.workerConfig.WorkerCount,
		RegisteredWorkerCount:        len(workers),
		BusyWorkerCount:              busyCount,
		IdleWorkerCount:              len(workers) - busyCount,
		RuntimeStartedAt:             snapshot.StartedAt,
		UptimeSeconds:                uptime,
		RuntimeMetricsResetOnRestart: true,
		Stopping:                     stopping,
		Workers:                      workers,
	}, nil
}

// 处理零分母并把比率限制在0-100之间，返回百分比值。
func percentage(numerator int64, denominator int64) float64 {
	if numerator <= 0 || denominator <= 0 {
		return 0
	}
	value := float64(numerator) / float64(denominator) * 100
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

// 移除控制字段、把换行符转空格，并按UTF-8 rune截断到最大长度512，返回安全的错误消息字符串。
func safeOpsErrorMessage(value string) string {
	value = strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == '\t' {
			return ' '
		}
		if r >= 0x20 && r != 0x7f {
			return r
		}
		return -1
	}, strings.TrimSpace(value))
	for len(value) > 512 {
		_, size := utf8.DecodeLastRuneInString(value)
		value = value[:len(value)-size]
	}
	return value
}
