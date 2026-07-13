package model

import "time"

// 项目文件的模型，包含文件路径、文件类型、内容hash值、文件大小等信息
type ProjectFile struct {
	ID          uint      `gorm:"primaryKey"`                      // 项目文件的唯一标识符，类型为uint，作为主键使用。
	ProjectID   uint      `gorm:"index;not null"`                  // 关联的项目ID，类型为uint，不能为空，并且在数据库中具有索引约束。
	FilePath    string    `gorm:"type:varchar(1024);not null"`     // 文件路径，类型为字符串，长度为1024个字符，不能为空。
	FileType    string    `gorm:"type:varchar(64)"`                // 文件类型，类型为字符串，长度为64个字符。
	ContentHash string    `gorm:"type:varchar(128);index"`         // 内容hash值，类型为字符串，长度为128个字符，并且在数据库中具有索引约束。
	Size        int64     `gorm:"not null;default:0"`              // 文件大小，类型为int64，不能为空，默认值为0。
	CreateAt    time.Time `gorm:"column:create_at;autoCreateTime"` // 创建时间，类型为time.Time。
	UpdateAt    time.Time `gorm:"column:update_at;autoUpdateTime"` // 更新时间，类型为time.Time。
}
