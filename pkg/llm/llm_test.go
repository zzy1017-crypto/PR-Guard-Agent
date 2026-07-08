package llm

import (
	"context"
	"strings"
	"testing"

	"pr-guard-agent/internal/config"
)

func TestMockGenerateParsesRiskReport(t *testing.T) {
	client := NewClient(config.LLMConfig{Provider: ProviderMock})

	raw, err := client.Generate(context.Background(), "analyze this diff")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	report, err := ParseRiskReport(raw)
	if err != nil {
		t.Fatalf("ParseRiskReport() error = %v", err)
	}
	if report.RiskLevel != "medium" {
		t.Fatalf("risk_level = %q, want medium", report.RiskLevel)
	}
	if len(report.RelatedFiles) == 0 || len(report.RelatedSymbols) == 0 {
		t.Fatal("mock report should include related files and symbols")
	}
}

func TestGenerateRejectsEmptyPrompt(t *testing.T) {
	client := NewClient(config.LLMConfig{Provider: ProviderMock})

	if _, err := client.Generate(context.Background(), " "); err == nil {
		t.Fatal("Generate() error = nil, want error")
	}
}

func TestBuildRiskAnalysisPrompt(t *testing.T) {
	prompt, err := BuildRiskAnalysisPrompt(RiskPromptInput{
		DiffText: "diff --git a/order.go b/order.go",
		ContextChunks: []ContextChunk{
			{
				ChunkID:    1,
				FilePath:   "internal/service/order_service.go",
				SymbolName: "OrderService.CreateOrder",
				SymbolType: "method",
				ChunkText:  "func (s *OrderService) CreateOrder() {}",
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildRiskAnalysisPrompt() error = %v", err)
	}

	for _, want := range []string{
		"Analyze only the given diff and context_chunks.",
		"related_files must be selected only from context_chunks.file_path.",
		"internal/service/order_service.go",
		"OrderService.CreateOrder",
		"Return JSON only.",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q", want)
		}
	}
}

func TestParseRiskReportExtractsFencedJSON(t *testing.T) {
	raw := "```json\n" + `{
  "risk_level": "high",
  "summary": "transaction risk",
  "affected_modules": [],
  "possible_risks": [],
  "suggested_tests": [],
  "related_files": [],
  "related_symbols": [],
  "confidence": 0.9
}` + "\n```"

	report, err := ParseRiskReport(raw)
	if err != nil {
		t.Fatalf("ParseRiskReport() error = %v", err)
	}
	if report.RiskLevel != "high" {
		t.Fatalf("risk_level = %q, want high", report.RiskLevel)
	}
}

func TestParseRiskReportRejectsInvalidConfidence(t *testing.T) {
	raw := `{
  "risk_level": "low",
  "summary": "ok",
  "affected_modules": [],
  "possible_risks": [],
  "suggested_tests": [],
  "related_files": [],
  "related_symbols": [],
  "confidence": 1.5
}`

	if _, err := ParseRiskReport(raw); err == nil {
		t.Fatal("ParseRiskReport() error = nil, want error")
	}
}
