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

func (r *DiffRepository) GetByID(diffID uint) (*model.DiffRecord, error) {
	return r.GetByIDWithContext(context.Background(), diffID)
}

func (r *DiffRepository) GetByIDWithContext(ctx context.Context, diffID uint) (*model.DiffRecord, error) {
	var diffRecord model.DiffRecord
	if err := r.db.WithContext(ctx).First(&diffRecord, diffID).Error; err != nil {
		return nil, err
	}
	return &diffRecord, nil
}

func (r *DiffRepository) Create(diffRecord *model.DiffRecord) error {
	return r.db.Create(diffRecord).Error
}
