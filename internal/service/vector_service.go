package service

import (
	"context"
	"fmt"

	"pr-guard-agent/internal/config"
	"pr-guard-agent/pkg/embedding"
	"pr-guard-agent/pkg/vector"
)

const (
	vectorTestPointID         uint64 = 1
	vectorTestProjectID       uint   = 1
	vectorTestChunkID         uint   = 1
	vectorTestCodeVersionHash        = "test_version"
	vectorTestContent                = "func Add(a int, b int) int { return a + b }"
)

type VectorService struct {
	qdrantCfg       config.QdrantConfig
	embeddingClient *embedding.Client
}

type VectorCollectionInitResult struct {
	CollectionName string `json:"collection_name"`
	VectorSize     int    `json:"vector_size"`
	Distance       string `json:"distance"`
}

type VectorTestUpsertResult struct {
	PointID         uint64 `json:"point_id"`
	ProjectID       uint   `json:"project_id"`
	CodeVersionHash string `json:"code_version_hash"`
	ChunkID         uint   `json:"chunk_id"`
	Dimension       int    `json:"dimension"`
}

type VectorTestSearchResult struct {
	ProjectID       uint                  `json:"project_id"`
	CodeVersionHash string                `json:"code_version_hash"`
	TopK            uint64                `json:"top_k"`
	Results         []vector.SearchResult `json:"results"`
}

// 注入Qdrant配置和Embedding Client。
func NewVectorService(qdrantCfg config.QdrantConfig, embeddingClient *embedding.Client) *VectorService {
	return &VectorService{
		qdrantCfg:       qdrantCfg,
		embeddingClient: embeddingClient,
	}
}

// 创建Client、确认Collection并返回配置摘要。
func (s *VectorService) InitCollection(ctx context.Context) (*VectorCollectionInitResult, error) {
	client, err := vector.NewClient(s.qdrantCfg)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	if err := client.EnsureCollection(ctx); err != nil {
		return nil, err
	}

	return &VectorCollectionInitResult{
		CollectionName: s.qdrantCfg.CollectionName,
		VectorSize:     s.qdrantCfg.VectorSize,
		Distance:       s.qdrantCfg.Distance,
	}, nil
}

// 为固定Add函数生成向量并写固定测试Point。
func (s *VectorService) TestUpsert(ctx context.Context) (*VectorTestUpsertResult, error) {
	vectorValues, err := s.embeddingClient.EmbedText(ctx, vectorTestContent)
	if err != nil {
		return nil, fmt.Errorf("generate test embedding failed: %w", err)
	}

	client, err := vector.NewClient(s.qdrantCfg)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	if err := client.EnsureCollection(ctx); err != nil {
		return nil, err
	}

	point := vector.ChunkPoint{
		ID:              vectorTestPointID,
		Vector:          vectorValues,
		ProjectID:       vectorTestProjectID,
		CodeVersionHash: vectorTestCodeVersionHash,
		ChunkID:         vectorTestChunkID,
		FilePath:        "internal/example/add.go",
		SymbolName:      "Add",
		SymbolType:      "function",
		StartLine:       1,
		EndLine:         1,
		ContentHash:     "test_content_hash",
	}

	if err := client.UpsertChunks(ctx, []vector.ChunkPoint{point}); err != nil {
		return nil, err
	}

	return &VectorTestUpsertResult{
		PointID:         point.ID,
		ProjectID:       point.ProjectID,
		CodeVersionHash: point.CodeVersionHash,
		ChunkID:         point.ChunkID,
		Dimension:       len(point.Vector),
	}, nil
}

// 用相同内容查询，验证Embedding->Qdrant搜索链路。
func (s *VectorService) TestSearch(ctx context.Context, topK uint64) (*VectorTestSearchResult, error) {
	if topK == 0 {
		topK = 5
	}

	vectorValues, err := s.embeddingClient.EmbedText(ctx, vectorTestContent)
	if err != nil {
		return nil, fmt.Errorf("generate test embedding failed: %w", err)
	}

	client, err := vector.NewClient(s.qdrantCfg)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	results, err := client.SearchTopK(ctx, vectorValues, vector.SearchFilter{
		ProjectID:       vectorTestProjectID,
		CodeVersionHash: vectorTestCodeVersionHash,
	}, topK)
	if err != nil {
		return nil, err
	}

	return &VectorTestSearchResult{
		ProjectID:       vectorTestProjectID,
		CodeVersionHash: vectorTestCodeVersionHash,
		TopK:            topK,
		Results:         results,
	}, nil
}
