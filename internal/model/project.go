package model

import "time"

// 上传的代码项目
type Project struct {
	ID              uint      `gorm:"primaryKey"`                      // 一次上传项目的唯一标识符，类型为uint，作为主键使用。
	Name            string    `gorm:"type:varchar(255);not null"`      // 项目名称，类型为字符串，长度为255个字符，不能为空。
	CodeVersionHash string    `gorm:"type:varchar(128);index"`         // 版本化索引和Redis缓存的key的核心字段。
	CreateAt        time.Time `gorm:"column:create_at;autoCreateTime"` // 创建时间，类型为time.Time。
	UpdateAt        time.Time `gorm:"column:update_at;autoUpdateTime"` // 更新时间，类型为time.Time。
}
