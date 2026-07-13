package model

import "time"

// 代码块模型，包含关联的项目ID、文件ID、文件路径、符号名称、符号类型、起始行、结束行、代码块文本内容、内容哈希值、代码版本哈希值、嵌入向量ID以及创建时间和更新时间等信息
type CodeChunk struct {
	ID              uint      `gorm:"primaryKey"`                      // 代码块的唯一标识符，类型为uint，作为主键使用。
	ProjectID       uint      `gorm:"index;not null"`                  // 关联的项目ID，类型为uint，不能为空，并且在数据库中具有索引约束。
	FileID          uint      `gorm:"index;not null"`                  // 关联的文件ID，类型为uint，不能为空，并且在数据库中具有索引约束。
	FilePath        string    `gorm:"type:varchar(1024);not null"`     // 文件路径，类型为字符串，长度为1024个字符，不能为空。
	SymbolName      string    `gorm:"type:varchar(255);index"`         // 符号名称，类型为字符串，长度为255个字符，并且在数据库中具有索引约束。
	SymbolType      string    `gorm:"type:varchar(64)"`                // 符号类型，类型为字符串，长度为64个字符。
	StartLine       int       `gorm:"not null;default:0"`              // 起始行，类型为int，不能为空，默认值为0。
	EndLine         int       `gorm:"not null;default:0"`              // 结束行，类型为int，不能为空，默认值为0。
	ChunkText       string    `gorm:"type:longtext"`                   // 代码块文本内容，类型为字符串，长度为长文本。
	ContentHash     string    `gorm:"type:varchar(128);index"`         // 内容哈希值，类型为字符串，长度为128个字符，并且在数据库中具有索引约束。
	CodeVersionHash string    `gorm:"type:varchar(128);index"`         // 代码版本哈希值，类型为字符串，长度为128个字符，并且在数据库中具有索引约束。
	EmbeddingID     string    `gorm:"type:varchar(255);index"`         // 嵌入向量ID，类型为字符串，长度为255个字符，并且在数据库中具有索引约束。
	CreateAt        time.Time `gorm:"column:create_at;autoCreateTime"` // 创建时间，类型为time.Time。
	UpdateAt        time.Time `gorm:"column:update_at;autoUpdateTime"` // 更新时间，类型为time.Time。
}
