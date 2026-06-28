package model

import "time"

// Go AST语义分块后的函数、方法、结构体、接口等代码块的模型
type CodeChunk struct {
	ID              uint      `gorm:"primaryKey"`
	ProjectID       uint      `gorm:"index;not null"`
	FileID          uint      `gorm:"index;not null"`
	FilePath        string    `gorm:"type:varchar(1024);not null"`
	SymbolName      string    `gorm:"type:varchar(255);index"`
	SymbolType      string    `gorm:"type:varchar(64)"`
	StartLine       int       `gorm:"not null;default:0"`
	EndLine         int       `gorm:"not null;default:0"`
	ChunkText       string    `gorm:"type:longtext"`
	ContentHash     string    `gorm:"type:varchar(128);index"`
	CodeVersionHash string    `gorm:"type:varchar(128);index"`
	EmbeddingID     string    `gorm:"type:varchar(255);index"`
	CreateAt        time.Time `gorm:"column:create_at;autoCreateTime"`
	UpdateAt        time.Time `gorm:"column:update_at;autoUpdateTime"`
}
