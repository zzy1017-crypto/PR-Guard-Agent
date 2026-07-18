package service

import (
	"context"

	"pr-guard-agent/pkg/embedding"
)

type EmbeddingService struct {
	client *embedding.Client
}

type EmbeddingTestResult struct {
	Dimension int  `json:"dimension"`
	Mock      bool `json:"mock"`
}

// 注入embedding客户端，创建EmbeddingService实例。
func NewEmbeddingService(client *embedding.Client) *EmbeddingService {
	return &EmbeddingService{client: client}
}

// 生成单文本向量，只返回维度和Mock标志
func (s *EmbeddingService) Test(ctx context.Context, text string) (*EmbeddingTestResult, error) {
	vector, err := s.client.EmbedText(ctx, text)
	if err != nil {
		return nil, err
	}

	return &EmbeddingTestResult{
		Dimension: len(vector),
		Mock:      s.client.IsMock(),
	}, nil
}
