package repository

import (
	"pr-guard-agent/internal/model"

	"gorm.io/gorm"
)

type ProjectFileRepository struct {
	db *gorm.DB
}

// 注入DB
func NewProjectFileRepository(db *gorm.DB) *ProjectFileRepository {
	return &ProjectFileRepository{db: db}
}

// 按路径排序加载文件；排序保证版本哈希计算和索引过程稳定。
func (r *ProjectFileRepository) ListByProjectID(projectID uint) ([]model.ProjectFile, error) {
	var files []model.ProjectFile
	err := r.db.Where("project_id = ?", projectID).Order("file_path ASC").Find(&files).Error
	return files, err
}

// 批量插入文件；空切片直接成功。
func (r *ProjectFileRepository) BatchCreate(files []model.ProjectFile) error {
	if len(files) == 0 {
		return nil
	}
	return r.db.Create(&files).Error
}
