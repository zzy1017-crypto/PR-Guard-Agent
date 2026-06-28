package model

import "time"

// 项目文件的模型，包含文件路径、文件类型、内容hash值、文件大小等信息
type ProjectFile struct {
	ID          uint      `gorm:"primaryKey"`
	ProjectID   uint      `gorm:"index;not null"`
	FilePath    string    `gorm:"type:varchar(1024);not null"`
	FileType    string    `gorm:"type:varchar(64)"`
	ContentHash string    `gorm:"type:varchar(128);index"`
	Size        int64     `gorm:"not null;default:0"`
	CreateAt    time.Time `gorm:"column:create_at;autoCreateTime"`
	UpdateAt    time.Time `gorm:"column:update_at;autoUpdateTime"`
}
