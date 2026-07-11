package llm

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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
	if report.RelatedFiles == nil || report.RelatedSymbols == nil {
		t.Fatal("mock report source arrays must be non-nil")
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

func TestParseRiskReportValid(t *testing.T) {
	raw := `模型分析结果如下：{"risk_level":"medium","summary":" transaction risk ","affected_modules":[" order ","","order"],"possible_risks":[],"suggested_tests":[],"related_files":["order.go","order.go"],"related_symbols":["CreateOrder"],"confidence":0.8} 请人工复核。`
	report, err := ParseRiskReport(raw)
	if err != nil {
		t.Fatalf("ParseRiskReport() error = %v", err)
	}
	if report.Summary != "transaction risk" || len(report.AffectedModules) != 1 || report.AffectedModules[0] != "order" {
		t.Fatalf("report was not normalized: %#v", report)
	}
	if len(report.RelatedFiles) != 1 {
		t.Fatalf("related_files = %#v, want deduplicated value", report.RelatedFiles)
	}
}

func TestParseRiskReportMarkdownJSON(t *testing.T) {
	raw := "```json\n" + validRiskReportJSON() + "\n```"
	if _, err := ParseRiskReport(raw); err != nil {
		t.Fatalf("ParseRiskReport() error = %v", err)
	}
}

func TestParseRiskReportInvalidJSON(t *testing.T) {
	_, err := ParseRiskReport("not json")
	if !errors.Is(err, ErrLLMInvalidJSON) {
		t.Fatalf("ParseRiskReport() error = %v, want ErrLLMInvalidJSON", err)
	}
}

func TestParseRiskReportInvalidRiskLevel(t *testing.T) {
	raw := strings.Replace(validRiskReportJSON(), `"medium"`, `"critical"`, 1)
	_, err := ParseRiskReport(raw)
	if !errors.Is(err, ErrLLMInvalidReport) {
		t.Fatalf("ParseRiskReport() error = %v, want ErrLLMInvalidReport", err)
	}
}

func TestParseRiskReportInvalidConfidence(t *testing.T) {
	raw := strings.Replace(validRiskReportJSON(), `0.8`, `-0.1`, 1)
	_, err := ParseRiskReport(raw)
	if !errors.Is(err, ErrLLMInvalidReport) {
		t.Fatalf("ParseRiskReport() error = %v, want ErrLLMInvalidReport", err)
	}
}

func TestValidateRiskReportSourcesValid(t *testing.T) {
	report := validRiskReportForSources()
	if err := ValidateRiskReportSources(report, validContextChunks()); err != nil {
		t.Fatalf("ValidateRiskReportSources() error = %v", err)
	}
}

func TestValidateRiskReportSourcesInventedFile(t *testing.T) {
	report := validRiskReportForSources()
	report.RelatedFiles = []string{"invented.go"}
	err := ValidateRiskReportSources(report, validContextChunks())
	if !errors.Is(err, ErrLLMInvalidReport) {
		t.Fatalf("ValidateRiskReportSources() error = %v, want ErrLLMInvalidReport", err)
	}
}

func TestValidateRiskReportSourcesInventedSymbol(t *testing.T) {
	report := validRiskReportForSources()
	report.RelatedSymbols = []string{"Invented.Symbol"}
	err := ValidateRiskReportSources(report, validContextChunks())
	if !errors.Is(err, ErrLLMInvalidReport) {
		t.Fatalf("ValidateRiskReportSources() error = %v, want ErrLLMInvalidReport", err)
	}
}

func TestMockLLMTimeout(t *testing.T) {
	client := NewClient(config.LLMConfig{
		Provider:    ProviderMock,
		MockMode:    "normal",
		MockDelayMS: 100,
	})
	client.timeout = 10 * time.Millisecond

	_, err := client.Generate(context.Background(), "analyze")
	if !errors.Is(err, ErrLLMTimeout) {
		t.Fatalf("Generate() error = %v, want ErrLLMTimeout", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Generate() error = %v, want context.DeadlineExceeded in chain", err)
	}
}

func validRiskReportJSON() string {
	return `{"risk_level":"medium","summary":"transaction risk","affected_modules":[],"possible_risks":[],"suggested_tests":[],"related_files":[],"related_symbols":[],"confidence":0.8}`
}

func validRiskReportForSources() *RiskReport {
	return &RiskReport{
		RelatedFiles:   []string{"internal/service/order.go"},
		RelatedSymbols: []string{"OrderService.CreateOrder"},
	}
}

func validContextChunks() []ContextChunk {
	return []ContextChunk{{
		FilePath:   "internal/service/order.go",
		SymbolName: "OrderService.CreateOrder",
	}}
}
