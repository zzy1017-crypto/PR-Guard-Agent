package llm

import (
	"encoding/json"
	"errors"
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
		return nil, errors.New("risk report raw output is empty")
	}

	candidates := []string{raw}
	if fenced, ok := extractFencedJSON(raw); ok {
		candidates = append([]string{fenced}, candidates...)
	}

	var lastErr error
	for _, candidate := range candidates {
		report, err := parseRiskReportJSON(candidate)
		if err == nil {
			return report, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("parse risk report failed: %w", lastErr)
}

func parseRiskReportJSON(raw string) (*RiskReport, error) {
	var report RiskReport
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &report); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
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
		return errors.New("risk_level must be one of low, medium, high")
	}

	report.Summary = strings.TrimSpace(report.Summary)
	if report.Summary == "" {
		return errors.New("summary is empty")
	}

	if report.AffectedModules == nil {
		return errors.New("affected_modules must not be nil")
	}
	if report.PossibleRisks == nil {
		return errors.New("possible_risks must not be nil")
	}
	if report.SuggestedTests == nil {
		return errors.New("suggested_tests must not be nil")
	}
	if report.RelatedFiles == nil {
		return errors.New("related_files must not be nil")
	}
	if report.RelatedSymbols == nil {
		return errors.New("related_symbols must not be nil")
	}
	if report.Confidence < 0 || report.Confidence > 1 {
		return errors.New("confidence must be between 0 and 1")
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
