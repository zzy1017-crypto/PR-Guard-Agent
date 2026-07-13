package model

import "time"

// 风险报告模型，包含关联的项目ID、diff ID、风险等级、摘要、受影响的模块、可能的风险、建议的测试、相关文件和符号、置信度以及原始JSON数据等信息
type RiskReport struct {
	ID              uint      `gorm:"primaryKey"`                      // 一份风险报告的唯一标识符，类型为uint，作为主键使用。
	ProjectID       uint      `gorm:"index;not null"`                  // 关联的项目ID，类型为uint，不能为空，并且在数据库中具有索引约束。
	DiffID          uint      `gorm:"index;not null"`                  // 关联的diff ID，类型为uint，不能为空，并且在数据库中具有索引约束。
	RiskLevel       string    `gorm:"type:varchar(64)"`                // 风险等级，类型为字符串，长度为64个字符。
	Summary         string    `gorm:"type:longtext"`                   // 摘要，类型为字符串，长度为长文本。
	AffectedModules string    `gorm:"type:json"`                       // 受影响的模块，类型为JSON格式。
	PossibleRisks   string    `gorm:"type:json"`                       // 可能的风险，类型为JSON格式。
	SuggestedTests  string    `gorm:"type:json"`                       // 建议的测试，类型为JSON格式。
	RelatedFiles    string    `gorm:"type:json"`                       // 相关文件，类型为JSON格式。
	RelatedSymbols  string    `gorm:"type:json"`                       // 相关符号，类型为JSON格式。
	Confidence      float64   `gorm:"not null;default:0"`              // 置信度，类型为float64，不能为空，默认值为0。
	RawJSON         string    `gorm:"type:longtext"`                   // 原始JSON数据，类型为字符串，长度为长文本。
	CreatedAt       time.Time `gorm:"autoCreateTime"`                  // 创建时间，类型为time.Time。
	UpdateAt        time.Time `gorm:"column:update_at;autoUpdateTime"` // 更新时间，类型为time.Time。
}
