package repository

import (
	"context"

	"pr-guard-agent/internal/model"

	"gorm.io/gorm"
)

type ProjectRepository struct {
	db *gorm.DB
}

func NewProjectRepository(db *gorm.DB) *ProjectRepository {
	return &ProjectRepository{db: db}
}

func (r *ProjectRepository) GetByID(projectID uint) (*model.Project, error) {
	return r.GetByIDWithContext(context.Background(), projectID)
}

func (r *ProjectRepository) GetByIDWithContext(ctx context.Context, projectID uint) (*model.Project, error) {
	var project model.Project
	if err := r.db.WithContext(ctx).First(&project, projectID).Error; err != nil {
		return nil, err
	}
	return &project, nil
}

func (r *ProjectRepository) Create(project *model.Project) error {
	return r.db.Create(project).Error
}

func (r *ProjectRepository) UpdateCodeVersionHash(projectID uint, codeVersionHash string) error {
	return r.db.Model(&model.Project{}).
		Where("id = ?", projectID).
		Update("code_version_hash", codeVersionHash).Error
}

func (r *ProjectRepository) DeleteByID(projectID uint) error {
	return r.db.Delete(&model.Project{}, projectID).Error
}
