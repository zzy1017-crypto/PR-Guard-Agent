package repository

import (
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
	var diffRecord model.DiffRecord
	if err := r.db.First(&diffRecord, diffID).Error; err != nil {
		return nil, err
	}
	return &diffRecord, nil
}

func (r *DiffRepository) Create(diffRecord *model.DiffRecord) error {
	return r.db.Create(diffRecord).Error
}
