package model

import "time"

// 一次git diff的记录，包含diff的hash值、diff文本内容以及关联的项目ID
type DiffRecord struct {
	ID        uint      `gorm:"primaryKey"`
	ProjectID uint      `gorm:"index;not null"`
	DiffHash  string    `gorm:"type:varchar(128);index"`
	DiffText  string    `gorm:"type:longtext"`
	CreateAt  time.Time `gorm:"column:create_at;autoCreateTime"`
	UpdateAt  time.Time `gorm:"column:update_at;autoUpdateTime"`
}
