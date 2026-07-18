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

// 兼容入口，使用后台Context；若需要取消或超时，请使用GetByIDWithContext。
func (r *ProjectRepository) GetByID(projectID uint) (*model.Project, error) {
	return r.GetByIDWithContext(context.Background(), projectID)
}

// 按ID加载Project，返回指针，若不存在则返回nil；空ID直接返回nil。
func (r *ProjectRepository) GetByIDWithContext(ctx context.Context, projectID uint) (*model.Project, error) {
	var project model.Project
	if err := r.db.WithContext(ctx).First(&project, projectID).Error; err != nil {
		return nil, err
	}
	return &project, nil
}

// 创建项目记录。
func (r *ProjectRepository) Create(project *model.Project) error {
	return r.db.Create(project).Error
}

// 更新项目的代码版本哈希。
func (r *ProjectRepository) UpdateCodeVersionHash(projectID uint, codeVersionHash string) error {
	return r.db.Model(&model.Project{}).
		Where("id = ?", projectID).
		Update("code_version_hash", codeVersionHash).Error
}

// 上传失败时清理项目。
func (r *ProjectRepository) DeleteByID(projectID uint) error {
	return r.db.Delete(&model.Project{}, projectID).Error
}
