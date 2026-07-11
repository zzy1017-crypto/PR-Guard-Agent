package service

import (
	"encoding/json"
	"testing"
	"time"

	reportcache "pr-guard-agent/pkg/cache"
	"pr-guard-agent/pkg/llm"
)

func TestNewReportServiceInjectsReportCache(t *testing.T) {
	want := reportcache.NewReportCache(nil, time.Hour, true)
	service := NewReportService(nil, nil, nil, want)

	if service.reportCache != want {
		t.Fatalf("reportCache = %p, want %p", service.reportCache, want)
	}
}

func TestBuildRiskReportModel(t *testing.T) {
	report := &llm.RiskReport{
		RiskLevel:       "medium",
		Summary:         "transaction risk",
		AffectedModules: []string{"order", "stock"},
		PossibleRisks:   []string{"stock may be deducted without order creation"},
		SuggestedTests:  []string{"test rollback when order creation fails"},
		RelatedFiles:    []string{"internal/service/order.go"},
		RelatedSymbols:  []string{"OrderService.CreateOrder"},
		Confidence:      0.8,
	}

	model, err := buildRiskReportModel(1, 2, report, `{"risk_level":"medium"}`)
	if err != nil {
		t.Fatalf("buildRiskReportModel() error = %v", err)
	}

	if model.ProjectID != 1 || model.DiffID != 2 || model.RiskLevel != "medium" {
		t.Fatalf("unexpected report identity fields: %#v", model)
	}
	if model.Summary != report.Summary || model.Confidence != report.Confidence {
		t.Fatalf("unexpected summary/confidence: %#v", model)
	}
	assertJSONStringSlice(t, "affected_modules", model.AffectedModules, report.AffectedModules)
	assertJSONStringSlice(t, "possible_risks", model.PossibleRisks, report.PossibleRisks)
	assertJSONStringSlice(t, "suggested_tests", model.SuggestedTests, report.SuggestedTests)
	assertJSONStringSlice(t, "related_files", model.RelatedFiles, report.RelatedFiles)
	assertJSONStringSlice(t, "related_symbols", model.RelatedSymbols, report.RelatedSymbols)
}

func TestBuildFallbackReport(t *testing.T) {
	retrieveResult := &RetrieveResult{
		RelatedFiles: []string{"internal/service/order.go"},
		RelatedSymbols: []RelatedSymbolResult{
			{SymbolName: "OrderService.CreateOrder"},
			{SymbolName: "OrderService.CreateOrder"},
			{SymbolName: ""},
		},
	}

	result := BuildFallbackReport(3, 7, retrieveResult, "LLM request timed out")
	if result.ReportID != 0 || result.ProjectID != 3 || result.DiffID != 7 {
		t.Fatalf("unexpected fallback identity: %#v", result)
	}
	if !result.Degraded || result.DegradedReason != "LLM request timed out" || result.Cached {
		t.Fatalf("unexpected fallback state: %#v", result)
	}
	if result.RiskLevel != "medium" || result.Confidence != 0.2 {
		t.Fatalf("unexpected fallback risk: %#v", result)
	}
	if result.AffectedModules == nil || len(result.AffectedModules) != 0 {
		t.Fatalf("AffectedModules = %#v, want non-nil empty slice", result.AffectedModules)
	}
	if len(result.RelatedFiles) != 1 || len(result.RelatedSymbols) != 1 {
		t.Fatalf("unexpected fallback sources: files=%#v symbols=%#v", result.RelatedFiles, result.RelatedSymbols)
	}
}

func assertJSONStringSlice(t *testing.T, field string, raw string, want []string) {
	t.Helper()

	var got []string
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("%s is not valid JSON: %v", field, err)
	}
	if len(got) != len(want) {
		t.Fatalf("%s length = %d, want %d", field, len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s[%d] = %q, want %q", field, i, got[i], want[i])
		}
	}
}
