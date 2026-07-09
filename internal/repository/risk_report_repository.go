package repository

import (
	"pr-guard-agent/internal/model"

	"gorm.io/gorm"
)

type RiskReportRepository struct {
	db *gorm.DB
}

func NewRiskReportRepository(db *gorm.DB) *RiskReportRepository {
	return &RiskReportRepository{db: db}
}

func (r *RiskReportRepository) Create(report *model.RiskReport) error {
	return r.db.Create(report).Error
}

func (r *RiskReportRepository) FindByID(id uint) (*model.RiskReport, error) {
	var report model.RiskReport
	if err := r.db.First(&report, id).Error; err != nil {
		return nil, err
	}
	return &report, nil
}
