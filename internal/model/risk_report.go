package model

import "time"

type RiskReport struct {
	ID              uint      `gorm:"primaryKey"`
	ProjectID       uint      `gorm:"index;not null"`
	DiffID          uint      `gorm:"index;not null"`
	RiskLevel       string    `gorm:"type:varchar(64)"`
	Summary         string    `gorm:"type:longtext"`
	AffectedModules string    `gorm:"type:json"`
	PossibleRisks   string    `gorm:"type:json"`
	SuggestedTests  string    `gorm:"type:json"`
	RelatedFiles    string    `gorm:"type:json"`
	RelatedSymbols  string    `gorm:"type:json"`
	Confidence      float64   `gorm:"not null;default:0"`
	RawJSON         string    `gorm:"type:longtext"`
	CreatedAt       time.Time `gorm:"autoCreateTime"`
	UpdateAt        time.Time `gorm:"column:update_at;autoUpdateTime"`
}
