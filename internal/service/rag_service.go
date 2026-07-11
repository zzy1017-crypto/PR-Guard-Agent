package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"pr-guard-agent/internal/config"
	"pr-guard-agent/internal/database"
	"pr-guard-agent/internal/model"
	"pr-guard-agent/internal/repository"
	"pr-guard-agent/pkg/embedding"
	"pr-guard-agent/pkg/vector"

	"gorm.io/gorm"
)

const (
	defaultRetrieveTopK      = 5
	maxRetrieveTopK          = 20
	maxContextChunkTextRunes = 3000
)

var (
	ErrDiffNotFound        = errors.New("diff not found")
	ErrDiffProjectMismatch = errors.New("diff does not belong to project")
	ErrDiffTextEmpty       = errors.New("diff text is empty")
)

type RAGService struct {
	db              *gorm.DB
	projectRepo     *repository.ProjectRepository
	diffRepo        *repository.DiffRepository
	codeChunkRepo   *repository.CodeChunkRepository
	qdrantCfg       config.QdrantConfig
	embeddingClient *embedding.Client
}

type RetrieveResult struct {
	ProjectID      uint                  `json:"project_id"`
	DiffID         uint                  `json:"diff_id"`
	TopK           int                   `json:"top_k"`
	RelatedFiles   []string              `json:"related_files"`
	RelatedSymbols []RelatedSymbolResult `json:"related_symbols"`
	ContextChunks  []ContextChunkResult  `json:"context_chunks"`
}

type RelatedSymbolResult struct {
	ChunkID    uint    `json:"chunk_id"`
	FilePath   string  `json:"file_path"`
	SymbolName string  `json:"symbol_name"`
	SymbolType string  `json:"symbol_type"`
	StartLine  int     `json:"start_line"`
	EndLine    int     `json:"end_line"`
	Score      float32 `json:"score"`
}

type ContextChunkResult struct {
	ChunkID    uint    `json:"chunk_id"`
	FilePath   string  `json:"file_path"`
	SymbolName string  `json:"symbol_name"`
	SymbolType string  `json:"symbol_type"`
	StartLine  int     `json:"start_line"`
	EndLine    int     `json:"end_line"`
	Score      float32 `json:"score"`
	ChunkText  string  `json:"chunk_text"`
}

func NewRAGService(db *gorm.DB, qdrantCfg config.QdrantConfig, embeddingClient *embedding.Client) *RAGService {
	if embeddingClient == nil {
		embeddingClient = embedding.NewClient(config.EmbeddingConfig{})
	}

	return &RAGService{
		db:              db,
		projectRepo:     repository.NewProjectRepository(db),
		diffRepo:        repository.NewDiffRepository(db),
		codeChunkRepo:   repository.NewCodeChunkRepository(db),
		qdrantCfg:       qdrantCfg,
		embeddingClient: embeddingClient,
	}
}

func RetrieveRelatedChunks(ctx context.Context, projectID uint, diffID uint, topK int) (*RetrieveResult, error) {
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		return nil, fmt.Errorf("load config failed: %w", err)
	}

	return NewRAGService(database.DB, cfg.Qdrant, embedding.NewClient(cfg.Embedding)).
		RetrieveRelatedChunks(ctx, projectID, diffID, topK)
}

func (s *RAGService) RetrieveRelatedChunks(ctx context.Context, projectID uint, diffID uint, topK int) (*RetrieveResult, error) {
	return s.retrieveRelatedChunks(ctx, projectID, diffID, topK)
}

func (s *RAGService) RetrieveRelatedChunksWithContext(ctx context.Context, projectID uint, diffID uint, topK int) (*RetrieveResult, error) {
	return s.retrieveRelatedChunks(ctx, projectID, diffID, topK)
}

func (s *RAGService) retrieveRelatedChunks(ctx context.Context, projectID uint, diffID uint, topK int) (*RetrieveResult, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("database is not initialized")
	}
	if ctx == nil {
		return nil, errors.New("rag context is nil")
	}
	topK = normalizeRetrieveTopK(topK)

	project, err := s.projectRepo.GetByIDWithContext(ctx, projectID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("query project failed: %w", err)
	}

	diff, err := s.diffRepo.GetByIDWithContext(ctx, diffID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDiffNotFound
		}
		return nil, fmt.Errorf("query diff failed: %w", err)
	}
	if diff.ProjectID != project.ID {
		return nil, ErrDiffProjectMismatch
	}

	diffText := strings.TrimPrefix(diff.DiffText, "\uFEFF")
	if strings.TrimSpace(diffText) == "" {
		return nil, ErrDiffTextEmpty
	}

	queryVector, err := s.embeddingClient.EmbedText(ctx, diffText)
	if err != nil {
		return nil, fmt.Errorf("embedding failed: %w", err)
	}

	vectorClient, err := vector.NewClient(s.qdrantCfg)
	if err != nil {
		return nil, err
	}
	defer vectorClient.Close()

	searchResults, err := vectorClient.SearchTopK(ctx, queryVector, vector.SearchFilter{
		ProjectID:       project.ID,
		CodeVersionHash: project.CodeVersionHash,
	}, uint64(topK))
	if err != nil {
		return nil, fmt.Errorf("qdrant search failed: %w", err)
	}

	chunksByID, err := s.loadChunksBySearchResults(ctx, searchResults, project)
	if err != nil {
		return nil, err
	}

	result := &RetrieveResult{
		ProjectID:      project.ID,
		DiffID:         diff.ID,
		TopK:           topK,
		RelatedFiles:   make([]string, 0),
		RelatedSymbols: make([]RelatedSymbolResult, 0, len(searchResults)),
		ContextChunks:  make([]ContextChunkResult, 0, len(searchResults)),
	}

	seenFiles := make(map[string]struct{})
	for _, searchResult := range searchResults {
		chunkID := searchResultChunkID(searchResult)
		chunk := chunksByID[chunkID]
		symbol := relatedSymbolFromSearchResult(searchResult, chunk)

		if symbol.FilePath != "" {
			if _, ok := seenFiles[symbol.FilePath]; !ok {
				seenFiles[symbol.FilePath] = struct{}{}
				result.RelatedFiles = append(result.RelatedFiles, symbol.FilePath)
			}
		}

		result.RelatedSymbols = append(result.RelatedSymbols, symbol)
		result.ContextChunks = append(result.ContextChunks, contextChunkFromSymbol(symbol, chunk.ChunkText))
	}

	return result, nil
}

func (s *RAGService) loadChunksBySearchResults(ctx context.Context, searchResults []vector.SearchResult, project *model.Project) (map[uint]model.CodeChunk, error) {
	chunkIDs := uniqueSearchResultChunkIDs(searchResults)
	if len(chunkIDs) == 0 {
		return map[uint]model.CodeChunk{}, nil
	}

	chunks, err := s.codeChunkRepo.ListByIDsWithContext(ctx, chunkIDs)
	if err != nil {
		return nil, fmt.Errorf("query code_chunks failed: %w", err)
	}

	chunksByID := make(map[uint]model.CodeChunk, len(chunks))
	for _, chunk := range chunks {
		chunksByID[chunk.ID] = chunk
	}

	for _, chunkID := range chunkIDs {
		chunk, ok := chunksByID[chunkID]
		if !ok {
			return nil, fmt.Errorf("code_chunk not found: %d", chunkID)
		}
		if chunk.ProjectID != project.ID || chunk.CodeVersionHash != project.CodeVersionHash {
			return nil, fmt.Errorf("code_chunk %d does not match current project version", chunkID)
		}
	}

	return chunksByID, nil
}

func normalizeRetrieveTopK(topK int) int {
	if topK <= 0 {
		return defaultRetrieveTopK
	}
	if topK > maxRetrieveTopK {
		return maxRetrieveTopK
	}
	return topK
}

func uniqueSearchResultChunkIDs(searchResults []vector.SearchResult) []uint {
	chunkIDs := make([]uint, 0, len(searchResults))
	seen := make(map[uint]struct{}, len(searchResults))
	for _, searchResult := range searchResults {
		chunkID := searchResultChunkID(searchResult)
		if chunkID == 0 {
			continue
		}
		if _, ok := seen[chunkID]; ok {
			continue
		}
		seen[chunkID] = struct{}{}
		chunkIDs = append(chunkIDs, chunkID)
	}
	return chunkIDs
}

func searchResultChunkID(searchResult vector.SearchResult) uint {
	if searchResult.ChunkID != 0 {
		return searchResult.ChunkID
	}
	return searchResult.ID
}

func relatedSymbolFromSearchResult(searchResult vector.SearchResult, chunk model.CodeChunk) RelatedSymbolResult {
	return RelatedSymbolResult{
		ChunkID:    chunk.ID,
		FilePath:   firstNonEmpty(searchResult.FilePath, chunk.FilePath),
		SymbolName: firstNonEmpty(searchResult.SymbolName, chunk.SymbolName),
		SymbolType: firstNonEmpty(searchResult.SymbolType, chunk.SymbolType),
		StartLine:  firstNonZero(searchResult.StartLine, chunk.StartLine),
		EndLine:    firstNonZero(searchResult.EndLine, chunk.EndLine),
		Score:      searchResult.Score,
	}
}

func contextChunkFromSymbol(symbol RelatedSymbolResult, chunkText string) ContextChunkResult {
	return ContextChunkResult{
		ChunkID:    symbol.ChunkID,
		FilePath:   symbol.FilePath,
		SymbolName: symbol.SymbolName,
		SymbolType: symbol.SymbolType,
		StartLine:  symbol.StartLine,
		EndLine:    symbol.EndLine,
		Score:      symbol.Score,
		ChunkText:  truncateRunes(chunkText, maxContextChunkTextRunes),
	}
}

func firstNonEmpty(primary string, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return fallback
}

func firstNonZero(primary int, fallback int) int {
	if primary != 0 {
		return primary
	}
	return fallback
}

func truncateRunes(text string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes])
}
