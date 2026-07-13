package model

import "time"

const (
	AnalysisTaskStatusPending   = "pending"
	AnalysisTaskStatusRunning   = "running"
	AnalysisTaskStatusSucceeded = "succeeded"
	AnalysisTaskStatusFailed    = "failed"
)

// AnalysisTask stores only the durable execution state and the sanitized
// AnalyzeResult JSON. Source, diff, prompt, context chunks and provider output
// intentionally remain outside this table.
type AnalysisTask struct {
	ID              uint64 `gorm:"primaryKey"`
	TaskKey         string `gorm:"type:char(64);not null;uniqueIndex"`
	ProjectID       uint   `gorm:"not null;index"`
	DiffID          uint   `gorm:"not null;index"`
	TopK            int    `gorm:"not null"`
	Status          string `gorm:"type:varchar(16);not null;default:pending;index:idx_analysis_tasks_status_created_at,priority:1;check:status IN ('pending','running','succeeded','failed')"`
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
	CreatedAt       time.Time `gorm:"autoCreateTime;index:idx_analysis_tasks_status_created_at,priority:2"`
	UpdateAt        time.Time `gorm:"column:update_at;autoUpdateTime"`
}
