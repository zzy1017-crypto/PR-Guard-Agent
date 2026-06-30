package repository

import (
	"pr-guard-agent/internal/model"

	"gorm.io/gorm"
)

type ProjectFileRepository struct {
	db *gorm.DB
}

func NewProjectFileRepository(db *gorm.DB) *ProjectFileRepository {
	return &ProjectFileRepository{db: db}
}

func (r *ProjectFileRepository) ListByProjectID(projectID uint) ([]model.ProjectFile, error) {
	var files []model.ProjectFile
	err := r.db.Where("project_id = ?", projectID).Order("file_path ASC").Find(&files).Error
	return files, err
}

func (r *ProjectFileRepository) BatchCreate(files []model.ProjectFile) error {
	if len(files) == 0 {
		return nil
	}
	return r.db.Create(&files).Error
}
