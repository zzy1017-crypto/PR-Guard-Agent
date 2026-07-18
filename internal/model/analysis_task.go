package model

import "time"

const (
	AnalysisTaskStatusPending   = "pending"
	AnalysisTaskStatusRunning   = "running"
	AnalysisTaskStatusSucceeded = "succeeded"
	AnalysisTaskStatusFailed    = "failed"
)

// task_key唯一，领取索引由status/next_run_at/created_at组成，领取时按status/next_run_at/created_at升序排序，领取后更新status/worker_id/started_at/next_run_at/attempt_count
// 定义了一个分析任务的模型，包含任务的唯一标识符、任务键、关联的项目ID和diff ID、任务状态、尝试次数、最大尝试次数、工作节点ID、报告ID、结果JSON、降级标志、错误码、错误信息、提交请求ID以及任务的开始时间、结束时间、下一次运行时间和最后一次失败时间等信息。
type AnalysisTask struct {
	ID              uint64 `gorm:"primaryKey"`
	TaskKey         string `gorm:"type:char(64);not null;uniqueIndex"`
	ProjectID       uint   `gorm:"not null;index:idx_analysis_tasks_project_created,priority:1"`
	DiffID          uint   `gorm:"not null;index:idx_analysis_tasks_diff_created,priority:1"`
	TopK            int    `gorm:"not null"`
	Status          string `gorm:"type:varchar(16);not null;default:pending;index:idx_analysis_tasks_claim,priority:1;check:status IN ('pending','running','succeeded','failed')"`
	AttemptCount    int    `gorm:"not null;default:0"`
	MaxAttempts     int    `gorm:"not null"`
	WorkerID        string `gorm:"type:varchar(128);not null;default:''"`
	ReportID        *uint  `gorm:"index"`
	ResultJSON      string `gorm:"type:longtext"`
	Degraded        bool   `gorm:"not null;default:false"`
	ErrorCode       string `gorm:"type:varchar(64);not null;default:''"`
	ErrorMessage    string `gorm:"type:varchar(512);not null;default:''"`
	SubmitRequestID string `gorm:"type:varchar(128);not null;default:''"`
	StartedAt       *time.Time
	FinishedAt      *time.Time
	NextRunAt       *time.Time `gorm:"index:idx_analysis_tasks_claim,priority:2"`
	LastFailedAt    *time.Time
	CreatedAt       time.Time `gorm:"autoCreateTime;index:idx_analysis_tasks_claim,priority:3;index:idx_analysis_tasks_created;index:idx_analysis_tasks_project_created,priority:2;index:idx_analysis_tasks_diff_created,priority:2"`
	UpdateAt        time.Time `gorm:"column:update_at;autoUpdateTime"`
}
