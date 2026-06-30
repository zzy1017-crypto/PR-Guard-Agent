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
