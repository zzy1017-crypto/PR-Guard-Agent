package model

import "time"

// 风险报告的模型，包含风险等级、受影响的模块、可能的风险、建议的测试、相关文件和符号等信息
type RiskReport struct {
	ID              uint      `gorm:"primaryKey"`
	ProjectID       uint      `gorm:"index;not null"`
	DiffID          uint      `gorm:"index;not null"`
	RiskLevel       string    `gorm:"type:varchar(64)"`
	AffectedModules string    `gorm:"type:json"`
	PossibleRisks   string    `gorm:"type:json"`
	SuggestedTests  string    `gorm:"type:json"`
	RelatedFiles    string    `gorm:"type:json"`     // 定位到文件
	RelatedSymbols  string    `gorm:"type:json"`     //定位到函数、方法、结构体、接口
	RawJSON         string    `gorm:"type:longtext"` //原始的风险报告JSON数据
	CreatedAt       time.Time `gorm:"autoCreateTime"`
	UpdateAt        time.Time `gorm:"column:update_at;autoUpdateTime"`
}
