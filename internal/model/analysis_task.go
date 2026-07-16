package model

import "time"

const (
	AnalysisTaskStatusPending   = "pending"
	AnalysisTaskStatusRunning   = "running"
	AnalysisTaskStatusSucceeded = "succeeded"
	AnalysisTaskStatusFailed    = "failed"
)

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
