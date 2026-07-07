package repository

import (
	"pr-guard-agent/internal/model"

	"gorm.io/gorm"
)

type CodeChunkRepository struct {
	db *gorm.DB
}

func NewCodeChunkRepository(db *gorm.DB) *CodeChunkRepository {
	return &CodeChunkRepository{db: db}
}

func (r *CodeChunkRepository) BatchCreate(chunks []model.CodeChunk) error {
	if len(chunks) == 0 {
		return nil
	}
	return r.db.Create(&chunks).Error
}

func (r *CodeChunkRepository) DeleteByProjectID(projectID uint) error {
	return r.db.Where("project_id = ?", projectID).Delete(&model.CodeChunk{}).Error
}

func (r *CodeChunkRepository) ListByIDs(chunkIDs []uint) ([]model.CodeChunk, error) {
	if len(chunkIDs) == 0 {
		return []model.CodeChunk{}, nil
	}

	var chunks []model.CodeChunk
	if err := r.db.Where("id IN ?", chunkIDs).Find(&chunks).Error; err != nil {
		return nil, err
	}
	return chunks, nil
}

func (r *CodeChunkRepository) UpdateEmbeddingID(chunkID uint, embeddingID string) error {
	return r.db.Model(&model.CodeChunk{}).
		Where("id = ?", chunkID).
		Update("embedding_id", embeddingID).Error
}
