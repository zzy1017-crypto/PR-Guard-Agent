package repository

import (
	"context"

	"pr-guard-agent/internal/model"

	"gorm.io/gorm"
)

type DiffRepository struct {
	db *gorm.DB
}

func NewDiffRepository(db *gorm.DB) *DiffRepository {
	return &DiffRepository{db: db}
}

// 兼容入口，使用后台Context；若需要取消或超时，请使用GetByIDWithContext。
func (r *DiffRepository) GetByID(diffID uint) (*model.DiffRecord, error) {
	return r.GetByIDWithContext(context.Background(), diffID)
}

// 按ID加载DiffRecord，返回指针，若不存在则返回nil；空ID直接返回nil。
func (r *DiffRepository) GetByIDWithContext(ctx context.Context, diffID uint) (*model.DiffRecord, error) {
	var diffRecord model.DiffRecord
	if err := r.db.WithContext(ctx).First(&diffRecord, diffID).Error; err != nil {
		return nil, err
	}
	return &diffRecord, nil
}

// 插入diff记录。
func (r *DiffRepository) Create(diffRecord *model.DiffRecord) error {
	return r.db.Create(diffRecord).Error
}
