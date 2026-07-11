package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

type RiskReport struct {
	RiskLevel       string   `json:"risk_level"`
	Summary         string   `json:"summary"`
	AffectedModules []string `json:"affected_modules"`
	PossibleRisks   []string `json:"possible_risks"`
	SuggestedTests  []string `json:"suggested_tests"`
	RelatedFiles    []string `json:"related_files"`
	RelatedSymbols  []string `json:"related_symbols"`
	Confidence      float64  `json:"confidence"`
}

func ParseRiskReport(raw string) (*RiskReport, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("%w: raw output is empty", ErrLLMInvalidJSON)
	}

	if fenced, ok := extractFencedJSON(raw); ok {
		raw = fenced
	}
	if start, end := strings.Index(raw, "{"), strings.LastIndex(raw, "}"); start >= 0 && end >= start {
		raw = raw[start : end+1]
	}
	return parseRiskReportJSON(raw)
}

// RarseRiskReport preserves the DAY16 specification's historical spelling.
func RarseRiskReport(raw string) (*RiskReport, error) {
	return ParseRiskReport(raw)
}

func parseRiskReportJSON(raw string) (*RiskReport, error) {
	var report RiskReport
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &report); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLLMInvalidJSON, err)
	}

	if err := validateRiskReport(&report); err != nil {
		return nil, err
	}
	return &report, nil
}

func validateRiskReport(report *RiskReport) error {
	report.RiskLevel = strings.ToLower(strings.TrimSpace(report.RiskLevel))
	switch report.RiskLevel {
	case "low", "medium", "high":
	default:
		return fmt.Errorf("%w: risk_level must be one of low, medium, high", ErrLLMInvalidReport)
	}

	report.Summary = strings.TrimSpace(report.Summary)
	if report.Summary == "" {
		return fmt.Errorf("%w: summary is empty", ErrLLMInvalidReport)
	}

	if report.AffectedModules == nil {
		return fmt.Errorf("%w: affected_modules must not be nil", ErrLLMInvalidReport)
	}
	if report.PossibleRisks == nil {
		return fmt.Errorf("%w: possible_risks must not be nil", ErrLLMInvalidReport)
	}
	if report.SuggestedTests == nil {
		return fmt.Errorf("%w: suggested_tests must not be nil", ErrLLMInvalidReport)
	}
	if report.RelatedFiles == nil {
		return fmt.Errorf("%w: related_files must not be nil", ErrLLMInvalidReport)
	}
	if report.RelatedSymbols == nil {
		return fmt.Errorf("%w: related_symbols must not be nil", ErrLLMInvalidReport)
	}
	if report.Confidence < 0 || report.Confidence > 1 {
		return fmt.Errorf("%w: confidence must be between 0 and 1", ErrLLMInvalidReport)
	}

	report.AffectedModules = normalizeStringSlice(report.AffectedModules)
	report.PossibleRisks = normalizeStringSlice(report.PossibleRisks)
	report.SuggestedTests = normalizeStringSlice(report.SuggestedTests)
	report.RelatedFiles = normalizeStringSlice(report.RelatedFiles)
	report.RelatedSymbols = normalizeStringSlice(report.RelatedSymbols)

	return nil
}

func normalizeStringSlice(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func ValidateRiskReportSources(report *RiskReport, contextChunks []ContextChunk) error {
	if report == nil {
		return fmt.Errorf("%w: report is nil", ErrLLMInvalidReport)
	}
	allowedFiles := make(map[string]struct{}, len(contextChunks))
	allowedSymbols := make(map[string]struct{}, len(contextChunks))
	for _, chunk := range contextChunks {
		if file := strings.TrimSpace(chunk.FilePath); file != "" {
			allowedFiles[file] = struct{}{}
		}
		if symbol := strings.TrimSpace(chunk.SymbolName); symbol != "" {
			allowedSymbols[symbol] = struct{}{}
		}
	}
	for _, file := range report.RelatedFiles {
		if _, ok := allowedFiles[file]; !ok {
			return fmt.Errorf("%w: related_file %q is not present in context_chunks", ErrLLMInvalidReport, file)
		}
	}
	for _, symbol := range report.RelatedSymbols {
		if _, ok := allowedSymbols[symbol]; !ok {
			return fmt.Errorf("%w: related_symbol %q is not present in context_chunks", ErrLLMInvalidReport, symbol)
		}
	}
	return nil
}

func extractFencedJSON(raw string) (string, bool) {
	start := strings.Index(raw, "```")
	if start < 0 {
		return "", false
	}

	afterStart := raw[start+3:]
	firstLineEnd := strings.IndexByte(afterStart, '\n')
	if firstLineEnd < 0 {
		return "", false
	}

	fenceLabel := strings.TrimSpace(afterStart[:firstLineEnd])
	if fenceLabel != "" && !strings.EqualFold(fenceLabel, "json") {
		return "", false
	}

	bodyAndRest := afterStart[firstLineEnd+1:]
	end := strings.Index(bodyAndRest, "```")
	if end < 0 {
		return "", false
	}

	return strings.TrimSpace(bodyAndRest[:end]), true
}
