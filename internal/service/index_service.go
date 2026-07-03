package service

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"pr-guard-agent/internal/database"
	"pr-guard-agent/internal/model"
	"pr-guard-agent/internal/repository"
	"pr-guard-agent/pkg/chunker"

	"gorm.io/gorm"
)

var ErrProjectNotFound = errors.New("project not found")

type IndexProjectResult struct {
	ProjectID      uint `json:"project_id"`
	ChunkCount     int  `json:"chunk_count"`
	GoChunkCount   int  `json:"go_chunk_count"`
	TextChunkCount int  `json:"text_chunk_count"`
}

type IndexService struct {
	db              *gorm.DB
	projectRepo     *repository.ProjectRepository
	projectFileRepo *repository.ProjectFileRepository
}

func NewIndexService(db *gorm.DB) *IndexService {
	return &IndexService{
		db:              db,
		projectRepo:     repository.NewProjectRepository(db),
		projectFileRepo: repository.NewProjectFileRepository(db),
	}
}

func IndexProject(projectID uint) (*IndexProjectResult, error) {
	return NewIndexService(database.DB).IndexProject(projectID)
}

func (s *IndexService) IndexProject(projectID uint) (*IndexProjectResult, error) {
	project, err := s.projectRepo.GetByID(projectID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("query project failed: %w", err)
	}

	files, err := s.projectFileRepo.ListByProjectID(project.ID)
	if err != nil {
		return nil, fmt.Errorf("query project files failed: %w", err)
	}

	result := &IndexProjectResult{ProjectID: project.ID}
	codeChunks := make([]model.CodeChunk, 0)
	for _, file := range files {
		fullPath := projectFilePath(project.ID, file.FilePath)

		if isGoFileType(file.FileType) {
			astChunks, err := chunker.ParserGoFileToChunks(fullPath, file.FilePath, project.CodeVersionHash)
			if err != nil {
				return nil, fmt.Errorf("generate ast chunks failed for %s: %w", file.FilePath, err)
			}
			result.GoChunkCount += len(astChunks)
			codeChunks = append(codeChunks, astChunksToCodeChunks(project.ID, file.ID, astChunks)...)
			continue
		}

		if isTextIndexFileType(file.FileType) {
			textChunks, err := chunker.SplitTextFileToChunks(fullPath, file.FilePath, project.CodeVersionHash)
			if err != nil {
				return nil, fmt.Errorf("generate text chunks failed for %s: %w", file.FilePath, err)
			}
			result.TextChunkCount += len(textChunks)
			codeChunks = append(codeChunks, textChunksToCodeChunks(project.ID, file.ID, textChunks)...)
		}
	}

	result.ChunkCount = len(codeChunks)
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
		return nil, err
	}

	return result, nil
}

func astChunksToCodeChunks(projectID uint, fileID uint, astChunks []chunker.ASTChunk) []model.CodeChunk {
	codeChunks := make([]model.CodeChunk, 0, len(astChunks))
	for _, astChunk := range astChunks {
		codeChunks = append(codeChunks, model.CodeChunk{
			ProjectID:       projectID,
			FileID:          fileID,
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
	return codeChunks
}

func textChunksToCodeChunks(projectID uint, fileID uint, textChunks []chunker.TextChunk) []model.CodeChunk {
	codeChunks := make([]model.CodeChunk, 0, len(textChunks))
	for _, textChunk := range textChunks {
		codeChunks = append(codeChunks, model.CodeChunk{
			ProjectID:       projectID,
			FileID:          fileID,
			FilePath:        textChunk.FilePath,
			SymbolName:      textChunk.SymbolName,
			SymbolType:      textChunk.SymbolType,
			StartLine:       textChunk.StartLine,
			EndLine:         textChunk.EndLine,
			ChunkText:       textChunk.ChunkText,
			ContentHash:     textChunk.ContentHash,
			CodeVersionHash: textChunk.CodeVersionHash,
		})
	}
	return codeChunks
}

func projectFilePath(projectID uint, relativePath string) string {
	normalized := strings.ReplaceAll(relativePath, "\\", "/")
	return filepath.Join("data", "projects", fmt.Sprintf("%d", projectID), filepath.FromSlash(normalized))
}

func isTextIndexFileType(fileType string) bool {
	switch normalizeFileType(fileType) {
	case "md", "yaml", "yml", "json", "sql", "mod", "sum", "lua":
		return true
	default:
		return false
	}
}

func normalizeFileType(fileType string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(fileType)), ".")
}
