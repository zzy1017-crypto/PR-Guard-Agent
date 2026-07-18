package repository

import (
	"context"

	"pr-guard-agent/internal/model"

	"gorm.io/gorm"
)

type CodeChunkRepository struct {
	db *gorm.DB
}

func NewCodeChunkRepository(db *gorm.DB) *CodeChunkRepository {
	return &CodeChunkRepository{db: db}
}

// 空切片直接成功，否则批量插入。
func (r *CodeChunkRepository) BatchCreate(chunks []model.CodeChunk) error {
	if len(chunks) == 0 {
		return nil
	}
	return r.db.Create(&chunks).Error
}

// 删除项目已有chunk。
func (r *CodeChunkRepository) DeleteByProjectID(projectID uint) error {
	return r.db.Where("project_id = ?", projectID).Delete(&model.CodeChunk{}).Error
}

// 无Context兼容入口。
func (r *CodeChunkRepository) ListByIDs(chunkIDs []uint) ([]model.CodeChunk, error) {
	return r.ListByIDsWithContext(context.Background(), chunkIDs)
}

// 按ID集合加载chunk，返回切片，顺序不保证；空切片直接返回空切片。
func (r *CodeChunkRepository) ListByIDsWithContext(ctx context.Context, chunkIDs []uint) ([]model.CodeChunk, error) {
	if len(chunkIDs) == 0 {
		return []model.CodeChunk{}, nil
	}

	var chunks []model.CodeChunk
	if err := r.db.WithContext(ctx).Where("id IN ?", chunkIDs).Find(&chunks).Error; err != nil {
		return nil, err
	}
	return chunks, nil
}

// 向MySQL回写Qdrant Point标识。
func (r *CodeChunkRepository) UpdateEmbeddingID(chunkID uint, embeddingID string) error {
	return r.db.Model(&model.CodeChunk{}).
		Where("id = ?", chunkID).
		Update("embedding_id", embeddingID).Error
}
