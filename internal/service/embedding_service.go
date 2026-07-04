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

func NewEmbeddingService(client *embedding.Client) *EmbeddingService {
	return &EmbeddingService{client: client}
}

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
