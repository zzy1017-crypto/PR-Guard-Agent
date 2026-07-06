package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"pr-guard-agent/internal/config"
	"pr-guard-agent/internal/database"
	"pr-guard-agent/internal/model"
	"pr-guard-agent/internal/repository"
	"pr-guard-agent/pkg/chunker"
	"pr-guard-agent/pkg/embedding"
	"pr-guard-agent/pkg/vector"

	"gorm.io/gorm"
)

var ErrProjectNotFound = errors.New("project not found")

type IndexProjectResult struct {
	ProjectID           uint   `json:"project_id"`
	ChunkCount          int    `json:"chunk_count"`
	GoChunkCount        int    `json:"go_chunk_count"`
	TextChunkCount      int    `json:"text_chunk_count"`
	EmbeddedCount       int    `json:"embedded_count"`
	QdrantUpsertedCount int    `json:"qdrant_upserted_count"`
	CollectionName      string `json:"collection_name"`
}

type IndexService struct {
	db              *gorm.DB
	projectRepo     *repository.ProjectRepository
	projectFileRepo *repository.ProjectFileRepository
	codeChunkRepo   *repository.CodeChunkRepository
	qdrantCfg       config.QdrantConfig
	embeddingClient *embedding.Client
}

func NewIndexService(db *gorm.DB, qdrantCfg config.QdrantConfig, embeddingClient *embedding.Client) *IndexService {
	if embeddingClient == nil {
		embeddingClient = embedding.NewClient(config.EmbeddingConfig{})
	}

	return &IndexService{
		db:              db,
		projectRepo:     repository.NewProjectRepository(db),
		projectFileRepo: repository.NewProjectFileRepository(db),
		codeChunkRepo:   repository.NewCodeChunkRepository(db),
		qdrantCfg:       qdrantCfg,
		embeddingClient: embeddingClient,
	}
}

func IndexProject(projectID uint) (*IndexProjectResult, error) {
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		return nil, fmt.Errorf("load config failed: %w", err)
	}

	return NewIndexService(database.DB, cfg.Qdrant, embedding.NewClient(cfg.Embedding)).IndexProject(projectID)
}

func (s *IndexService) IndexProject(projectID uint) (*IndexProjectResult, error) {
	return s.IndexProjectWithContext(context.Background(), projectID)
}

func (s *IndexService) IndexProjectWithContext(ctx context.Context, projectID uint) (*IndexProjectResult, error) {
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
	if err := validateCodeChunksForEmbedding(codeChunks); err != nil {
		return nil, err
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
		return nil, err
	}

	vectorClient, err := vector.NewClient(s.qdrantCfg)
	if err != nil {
		return nil, err
	}
	defer vectorClient.Close()

	result.CollectionName = vectorClient.CollectionName()
	if err := vectorClient.EnsureCollection(ctx); err != nil {
		return nil, fmt.Errorf("ensure qdrant collection failed: %w", err)
	}

	if err := s.embedAndUpsertChunks(ctx, codeChunks, vectorClient, result); err != nil {
		return nil, err
	}

	return result, nil
}

func (s *IndexService) embedAndUpsertChunks(ctx context.Context, codeChunks []model.CodeChunk, vectorClient *vector.Client, result *IndexProjectResult) error {
	batchSize := s.embeddingClient.BatchSize()
	if batchSize <= 0 {
		batchSize = len(codeChunks)
	}

	for start := 0; start < len(codeChunks); start += batchSize {
		end := start + batchSize
		if end > len(codeChunks) {
			end = len(codeChunks)
		}

		batchChunks := codeChunks[start:end]
		texts := make([]string, 0, len(batchChunks))
		for _, chunk := range batchChunks {
			texts = append(texts, chunk.ChunkText)
		}

		vectors, err := s.embeddingClient.EmbedTexts(ctx, texts)
		if err != nil {
			return fmt.Errorf("generate embeddings failed for chunk batch %d-%d: %w", start, end-1, err)
		}
		if len(vectors) != len(batchChunks) {
			return fmt.Errorf("embedding count mismatch for chunk batch %d-%d: got %d, want %d", start, end-1, len(vectors), len(batchChunks))
		}

		points := codeChunksToPoints(batchChunks, vectors)
		if err := vectorClient.UpsertChunks(ctx, points); err != nil {
			return fmt.Errorf("upsert qdrant chunks failed for chunk batch %d-%d: %w", start, end-1, err)
		}

		for _, chunk := range batchChunks {
			embeddingID := strconv.FormatUint(uint64(chunk.ID), 10)
			if err := s.codeChunkRepo.UpdateEmbeddingID(chunk.ID, embeddingID); err != nil {
				return fmt.Errorf("update embedding_id failed for chunk %d: %w", chunk.ID, err)
			}
		}

		result.EmbeddedCount += len(vectors)
		result.QdrantUpsertedCount += len(points)
	}

	return nil
}

func validateCodeChunksForEmbedding(codeChunks []model.CodeChunk) error {
	for i, chunk := range codeChunks {
		if strings.TrimSpace(chunk.ChunkText) == "" {
			return fmt.Errorf("chunk_text is empty at index %d for file %s symbol %s", i, chunk.FilePath, chunk.SymbolName)
		}
	}
	return nil
}

func codeChunksToPoints(codeChunks []model.CodeChunk, vectors [][]float32) []vector.ChunkPoint {
	points := make([]vector.ChunkPoint, 0, len(codeChunks))
	for i, chunk := range codeChunks {
		points = append(points, vector.ChunkPoint{
			ID:              uint64(chunk.ID),
			Vector:          vectors[i],
			ProjectID:       chunk.ProjectID,
			CodeVersionHash: chunk.CodeVersionHash,
			ChunkID:         chunk.ID,
			FilePath:        chunk.FilePath,
			SymbolName:      chunk.SymbolName,
			SymbolType:      chunk.SymbolType,
			StartLine:       chunk.StartLine,
			EndLine:         chunk.EndLine,
			ContentHash:     chunk.ContentHash,
		})
	}
	return points
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
