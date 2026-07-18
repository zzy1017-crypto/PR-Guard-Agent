package service

import (
	"context"
	"errors"
	"fmt"

	"pr-guard-agent/pkg/llm"
)

type LLMService struct {
	client *llm.Client
}

type RiskTestResult struct {
	Report    *llm.RiskReport `json:"report"`
	RawOutput string          `json:"raw_output"`
}

// 注入LLM客户端，创建LLMService实例。
func NewLLMService(client *llm.Client) *LLMService {
	return &LLMService{client: client}
}

// 用内置订单/库存场景构造Prompt，调用LLM生成风险分析报告，并返回报告和原始输出。
func (s *LLMService) TestRiskReport(ctx context.Context) (*RiskTestResult, error) {
	if s == nil || s.client == nil {
		return nil, errors.New("llm service is not initialized")
	}

	prompt, err := llm.BuildRiskAnalysisPrompt(llm.RiskPromptInput{
		DiffText: testRiskDiffText(),
		ContextChunks: []llm.ContextChunk{
			{
				ChunkID:    1,
				FilePath:   "internal/service/order_service.go",
				SymbolName: "OrderService.CreateOrder",
				SymbolType: "method",
				StartLine:  18,
				EndLine:    64,
				Score:      0.91,
				ChunkText:  "func (s *OrderService) CreateOrder(ctx context.Context, req CreateOrderRequest) error { if err := s.stockService.DeductStock(ctx, req.SKU, req.Quantity); err != nil { return err }; return s.orderRepo.Create(ctx, req) }",
			},
			{
				ChunkID:    2,
				FilePath:   "internal/service/stock_service.go",
				SymbolName: "StockService.DeductStock",
				SymbolType: "method",
				StartLine:  22,
				EndLine:    58,
				Score:      0.86,
				ChunkText:  "func (s *StockService) DeductStock(ctx context.Context, sku string, quantity int) error { stock, err := s.repo.GetBySKU(ctx, sku); if err != nil { return err }; if stock.Available < quantity { return ErrInsufficientStock }; stock.Available -= quantity; return s.repo.Update(ctx, stock) }",
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("build risk analysis prompt failed: %w", err)
	}

	rawOutput, err := s.client.Generate(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("generate risk report failed: %w", err)
	}

	report, err := llm.ParseRiskReport(rawOutput)
	if err != nil {
		return nil, err
	}

	return &RiskTestResult{
		Report:    report,
		RawOutput: rawOutput,
	}, nil
}

// 返回固定测试diff，保证Mock/Provider联调可重复。
func testRiskDiffText() string {
	return `diff --git a/internal/service/order_service.go b/internal/service/order_service.go
index 1a2b3c4..5d6e7f8 100644
--- a/internal/service/order_service.go
+++ b/internal/service/order_service.go
@@ -20,6 +20,10 @@ func (s *OrderService) CreateOrder(ctx context.Context, req CreateOrderRequest)
+	if err := s.stockService.DeductStock(ctx, req.SKU, req.Quantity); err != nil {
+		return err
+	}
+
 	return s.orderRepo.Create(ctx, req)
 }`
}
