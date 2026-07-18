package repository

import (
	"context"

	"pr-guard-agent/internal/model"

	"gorm.io/gorm"
)

type RiskReportRepository struct {
	db *gorm.DB
}

func NewRiskReportRepository(db *gorm.DB) *RiskReportRepository {
	return &RiskReportRepository{db: db}
}

// 插入正常的风险报告记录。
func (r *RiskReportRepository) Create(report *model.RiskReport) error {
	return r.db.Create(report).Error
}

// 插入正常的风险报告记录，带Context。
func (r *RiskReportRepository) CreateWithContext(ctx context.Context, report *model.RiskReport) error {
	return r.db.WithContext(ctx).Create(report).Error
}

// 按ID查询报告
func (r *RiskReportRepository) FindByID(id uint) (*model.RiskReport, error) {
	var report model.RiskReport
	if err := r.db.First(&report, id).Error; err != nil {
		return nil, err
	}
	return &report, nil
}
