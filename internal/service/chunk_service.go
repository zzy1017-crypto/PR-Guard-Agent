package service

import (
	"fmt"
	"path/filepath"
	"strings"

	"pr-guard-agent/internal/model"
	"pr-guard-agent/internal/repository"
	"pr-guard-agent/pkg/chunker"

	"gorm.io/gorm"
)

type ChunkService struct {
	db              *gorm.DB
	projectRepo     *repository.ProjectRepository
	projectFileRepo *repository.ProjectFileRepository
	codeChunkRepo   *repository.CodeChunkRepository
}

// 装配项目、文件和chunk repository.
func NewChunkService(db *gorm.DB) *ChunkService {
	return &ChunkService{
		db:              db,
		projectRepo:     repository.NewProjectRepository(db),
		projectFileRepo: repository.NewProjectFileRepository(db),
		codeChunkRepo:   repository.NewCodeChunkRepository(db),
	}
}

// 只遍历Go文件生成AST Chunk,事务替换项目的MySQL Chunk。
func (s *ChunkService) GenerateASTChunks(projectID uint) (int, error) {
	project, err := s.projectRepo.GetByID(projectID)
	if err != nil {
		return 0, fmt.Errorf("query project failed: %w", err)
	}

	files, err := s.projectFileRepo.ListByProjectID(projectID)
	if err != nil {
		return 0, fmt.Errorf("query project files failed: %w", err)
	}

	projectDir := filepath.Join("data", "projects", fmt.Sprintf("%d", project.ID))
	codeChunks := make([]model.CodeChunk, 0)
	for _, file := range files {
		if !isGoFileType(file.FileType) {
			continue
		}

		fullPath := filepath.Join(projectDir, filepath.FromSlash(strings.ReplaceAll(file.FilePath, "\\", "/")))
		astChunks, err := chunker.ParserGoFileToChunks(fullPath, file.FilePath, project.CodeVersionHash)
		if err != nil {
			return 0, fmt.Errorf("generate ast chunks failed for %s: %w", file.FilePath, err)
		}

		for _, astChunk := range astChunks {
			codeChunks = append(codeChunks, model.CodeChunk{
				ProjectID:       project.ID,
				FileID:          file.ID,
				FilePath:        astChunk.FilePath,
				SymbolName:      astChunk.SymbolName,
				SymbolType:      astChunk.SymbolType,
				StartLine:       astChunk.StartLine,
				EndLine:         astChunk.EndLine,
				ChunkText:       astChunk.ChunkText,
				ContentHash:     astChunk.ContentHash,
				CodeVersionHash: astChunk.CodeVersionHash,
			})
		}
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		codeChunkRepo := repository.NewCodeChunkRepository(tx)
		if err := codeChunkRepo.DeleteByProjectID(project.ID); err != nil {
			return fmt.Errorf("delete old code chunks failed: %w", err)
		}
		if err := codeChunkRepo.BatchCreate(codeChunks); err != nil {
			return fmt.Errorf("save code chunks failed: %w", err)
		}
		return nil
	}); err != nil {
		return 0, err
	}

	return len(codeChunks), nil
}

// 判断文件类型是否为Go文件，支持"go"和".go"两种形式。
func isGoFileType(fileType string) bool {
	normalized := strings.ToLower(strings.TrimSpace(fileType))
	return normalized == "go" || normalized == ".go"
}
