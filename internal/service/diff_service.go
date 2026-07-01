package service

import (
	"errors"
	"fmt"
	"strings"

	"pr-guard-agent/internal/model"
	"pr-guard-agent/internal/repository"
	projecthash "pr-guard-agent/pkg/hash"
	"pr-guard-agent/pkg/parser"

	"gorm.io/gorm"
)

type DiffService struct {
	projectRepo *repository.ProjectRepository
	diffRepo    *repository.DiffRepository
}

type UploadDiffResult struct {
	DiffID       uint              `json:"diff_id"`
	ProjectID    uint              `json:"project_id"`
	DiffHash     string            `json:"diff_hash"`
	ChangedFiles []parser.DiffFile `json:"changed_files"`
}

func NewDiffService(db *gorm.DB) *DiffService {
	return &DiffService{
		projectRepo: repository.NewProjectRepository(db),
		diffRepo:    repository.NewDiffRepository(db),
	}
}

func (s *DiffService) UploadDiff(projectID uint, diffText string) (*UploadDiffResult, error) {
	diffText = strings.TrimPrefix(diffText, "\uFEFF")
	if strings.TrimSpace(diffText) == "" {
		return nil, fmt.Errorf("diff text is empty")
	}

	project, err := s.projectRepo.GetByID(projectID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("project not found")
		}
		return nil, fmt.Errorf("query project failed: %w", err)
	}

	changedFiles, err := parser.ParseDiff(diffText)
	if err != nil {
		return nil, fmt.Errorf("parse diff failed: %w", err)
	}

	diffHash := projecthash.SHA256String(diffText)
	diffRecord := &model.DiffRecord{
		ProjectID: project.ID,
		DiffHash:  diffHash,
		DiffText:  diffText,
	}
	if err := s.diffRepo.Create(diffRecord); err != nil {
		return nil, fmt.Errorf("save diff record failed: %w", err)
	}

	return &UploadDiffResult{
		DiffID:       diffRecord.ID,
		ProjectID:    project.ID,
		DiffHash:     diffHash,
		ChangedFiles: changedFiles,
	}, nil
}
