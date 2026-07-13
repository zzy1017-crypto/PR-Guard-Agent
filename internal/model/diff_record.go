package model

import "time"

// 一次git diff的记录，包含diff的hash值、diff文本内容以及关联的项目ID
type DiffRecord struct {
	ID        uint      `gorm:"primaryKey"`                      // 一次上传diff记录的唯一标识符，类型为uint，作为主键使用。
	ProjectID uint      `gorm:"index;not null"`                  // 关联的项目ID，类型为uint，不能为空，并且在数据库中具有索引约束。
	DiffHash  string    `gorm:"type:varchar(128);index"`         // diff的hash值，类型为字符串，长度为128个字符，并且在数据库中具有索引约束。
	DiffText  string    `gorm:"type:longtext"`                   // diff文本内容，类型为字符串，长度为长文本。
	CreateAt  time.Time `gorm:"column:create_at;autoCreateTime"` // 创建时间，类型为time.Time。
	UpdateAt  time.Time `gorm:"column:update_at;autoUpdateTime"` // 更新时间，类型为time.Time。
}
