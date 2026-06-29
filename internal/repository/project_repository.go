package repository

import (
	"pr-guard-agent/internal/model"

	"gorm.io/gorm"
)

// ProjectRepository 提供与项目相关的数据库操作，包括创建项目、更新代码版本哈希值和删除项目记录
type ProjectRepository struct {
	db *gorm.DB
}

// NewProjectRepository 创建一个新的ProjectRepository实例，接收一个数据库连接对象，并返回该实例
func NewProjectRepository(db *gorm.DB) *ProjectRepository {
	return &ProjectRepository{db: db}
}

// Create 创建一个新的项目记录，将传入的Project对象保存到数据库中，并返回可能的错误信息
func (r *ProjectRepository) Create(project *model.Project) error {
	return r.db.Create(project).Error
}

// UpdateCodeVersionHash 更新指定项目的代码版本哈希值，接收项目ID和新的代码版本哈希值，并将其更新到数据库中，如果更新失败，则返回错误信息
func (r *ProjectRepository) UpdateCodeVersionHash(projectID uint, codeVersionHash string) error {
	return r.db.Model(&model.Project{}).
		Where("id = ?", projectID).
		Update("code_version_hash", codeVersionHash).Error
}

// DeleteByID 删除指定项目ID的项目记录，如果删除失败，则返回错误信息
func (r *ProjectRepository) DeleteByID(projectID uint) error {
	return r.db.Delete(&model.Project{}, projectID).Error
}
