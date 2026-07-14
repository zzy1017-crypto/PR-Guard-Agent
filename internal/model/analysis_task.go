package model

import "time"

const (
	AnalysisTaskStatusPending   = "pending"   // 任务处于等待状态，尚未开始执行
	AnalysisTaskStatusRunning   = "running"   // 任务正在执行中
	AnalysisTaskStatusSucceeded = "succeeded" // 任务已成功完成
	AnalysisTaskStatusFailed    = "failed"    // 任务执行失败
)

// 分析任务模型，包含任务的唯一标识符、任务键、关联的项目ID、关联的差异ID、TopK值、任务状态、尝试次数、最大尝试次数、工作节点ID、报告ID、结果JSON表示、降级处理标志、错误码、错误信息、提交请求ID以及任务的开始时间和结束时间等信息
type AnalysisTask struct {
	ID              uint64     `gorm:"primaryKey"`                                                                                                                                     // 一次异步分析任务的唯一标识符，类型为uint64，作为主键使用。
	TaskKey         string     `gorm:"type:char(64);not null;uniqueIndex"`                                                                                                             // 任务的唯一键，用于标识任务，类型为字符串，长度为64个字符，不能为空，并且在数据库中具有唯一索引约束。
	ProjectID       uint       `gorm:"not null;index"`                                                                                                                                 // 关联的项目ID，类型为uint，不能为空，并且在数据库中具有索引约束。
	DiffID          uint       `gorm:"not null;index"`                                                                                                                                 // 关联的差异ID，类型为uint，不能为空，并且在数据库中具有索引约束。
	TopK            int        `gorm:"not null"`                                                                                                                                       // 任务的TopK值，类型为int，不能为空。
	Status          string     `gorm:"type:varchar(16);not null;default:pending;index:idx_analysis_tasks_claim,priority:1;check:status IN ('pending','running','succeeded','failed')"` // 任务状态，类型为字符串，长度为16个字符，不能为空，默认值为pending，并且在数据库中具有索引约束。
	AttemptCount    int        `gorm:"not null;default:0"`                                                                                                                             // 任务尝试次数，类型为int，不能为空，默认值为0。
	MaxAttempts     int        `gorm:"not null"`                                                                                                                                       // 任务最大尝试次数，类型为int，不能为空。
	WorkerID        string     `gorm:"type:varchar(128);not null;default:''"`                                                                                                          // 执行任务的工作节点ID，类型为字符串，长度为128个字符，不能为空，默认值为空字符串。
	ReportID        *uint      `gorm:"index"`                                                                                                                                          // 生成的报告ID，类型为指针，指向uint类型。
	ResultJSON      string     `gorm:"type:longtext"`                                                                                                                                  // 任务结果的JSON表示，类型为字符串，长度为长文本。
	Degraded        bool       `gorm:"not null;default:false"`                                                                                                                         // 是否降级处理，类型为bool，不能为空，默认值为false。
	ErrorCode       string     `gorm:"type:varchar(64);not null;default:''"`                                                                                                           // 错误码，类型为字符串，长度为64个字符，不能为空，默认值为空字符串。
	ErrorMessage    string     `gorm:"type:varchar(512);not null;default:''"`                                                                                                          // 错误信息，类型为字符串，长度为512个字符，不能为空，默认值为空字符串。
	SubmitRequestID string     `gorm:"type:varchar(128);not null;default:''"`                                                                                                          // 提交请求ID，类型为字符串，长度为128个字符，不能为空，默认值为空字符串。
	StartedAt       *time.Time // 任务开始时间，类型为*time.Time。
	FinishedAt      *time.Time // 任务结束时间，类型为*time.Time。
	NextRunAt       *time.Time `gorm:"index:idx_analysis_tasks_claim,priority:2"`
	LastFailedAt    *time.Time
	CreatedAt       time.Time `gorm:"autoCreateTime;index:idx_analysis_tasks_claim,priority:3"` // 任务创建时间，类型为time.Time。
	UpdateAt        time.Time `gorm:"column:update_at;autoUpdateTime"`                          // 任务更新时间，类型为time.Time。
}
