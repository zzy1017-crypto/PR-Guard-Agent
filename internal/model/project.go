package model

import "time"

// 上传的代码项目
type Project struct {
	ID              uint      `gorm:"primaryKey"`
	Name            string    `gorm:"type:varchar(255);not null"`
	CodeVersionHash string    `gorm:"type:varchar(128);index"` //版本化索引和Redis缓存的key的核心字段
	CreateAt        time.Time `gorm:"column:create_at;autoCreateTime"`
	UpdateAt        time.Time `gorm:"column:update_at;autoUpdateTime"`
}
