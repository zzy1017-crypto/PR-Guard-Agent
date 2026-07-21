package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

// 定义报告包括的字段。
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

// ParseRiskReport 解析 LLM 输出的风险报告 JSON 字符串，并返回 RiskReport 结构体。
// 它会自动提取 fenced JSON（```json ... ```）中的内容，并验证报告的字段是否符合要求。
// 如果报告无效或无法解析，将返回 ErrLLMInvalidReport 或 ErrLLMInvalidJSON 错误。
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

// RarseRiskReport函数是ParseRiskReport的别名，提供了相同的功能。
func RarseRiskReport(raw string) (*RiskReport, error) {
	return ParseRiskReport(raw)
}

// parseRiskReportJSON 解析 JSON 字符串为 RiskReport，并验证字段。
func parseRiskReportJSON(raw string) (*RiskReport, error) {
	var report RiskReport
	// 1. 尝试解析 JSON 字符串为 RiskReport 结构体。
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &report); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLLMInvalidJSON, err)
	}

	// 2. 验证报告的字段是否符合要求。
	if err := validateRiskReport(&report); err != nil {
		return nil, err
	}
	return &report, nil
}

// validateRiskReport 验证 RiskReport 的字段是否符合要求。
// 它会检查 risk_level 是否为 low、medium 或 high，
// summary 是否为空，confidence 是否在 0 到 1 之间，
// 并确保所有切片字段不为 nil。
// 同时，它会对切片字段进行去重和清理空白字符。
func validateRiskReport(report *RiskReport) error {
	report.RiskLevel = strings.ToLower(strings.TrimSpace(report.RiskLevel))
	// 1. 验证 risk_level 字段是否为 low、medium 或 high。
	switch report.RiskLevel {
	case "low", "medium", "high":
	default:
		return fmt.Errorf("%w: risk_level must be one of low, medium, high", ErrLLMInvalidReport)
	}

	// 2. 验证 summary 字段是否为空，并清理空白字符。
	report.Summary = strings.TrimSpace(report.Summary)
	if report.Summary == "" {
		return fmt.Errorf("%w: summary is empty", ErrLLMInvalidReport)
	}

	// 3. 验证 confidence 字段是否在 0 到 1 之间，并确保所有切片字段不为 nil。
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

	// 4. 对切片字段进行去重和清理空白字符。
	report.AffectedModules = normalizeStringSlice(report.AffectedModules)
	report.PossibleRisks = normalizeStringSlice(report.PossibleRisks)
	report.SuggestedTests = normalizeStringSlice(report.SuggestedTests)
	report.RelatedFiles = normalizeStringSlice(report.RelatedFiles)
	report.RelatedSymbols = normalizeStringSlice(report.RelatedSymbols)

	return nil
}

// normalizeStringSlice 对字符串切片进行去重和清理空白字符
func normalizeStringSlice(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	//遍历切片中的每个字符串，去除空白字符并检查是否已存在于结果中。
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

// 验证 RiskReport 中的 related_files 和 related_symbols 是否存在于 context_chunks 中。
// 如果存在不在 context_chunks 中的文件或符号，将返回 ErrLLMInvalidReport 错误。
func ValidateRiskReportSources(report *RiskReport, contextChunks []ContextChunk) error {
	if report == nil {
		return fmt.Errorf("%w: report is nil", ErrLLMInvalidReport)
	}
	// 1. 创建两个集合，分别存储 context_chunks 中的文件路径和符号名称。
	allowedFiles := make(map[string]struct{}, len(contextChunks))
	allowedSymbols := make(map[string]struct{}, len(contextChunks))
	// 2. 遍历 context_chunks，将文件路径和符号名称添加到集合中。
	for _, chunk := range contextChunks {
		if file := strings.TrimSpace(chunk.FilePath); file != "" {
			allowedFiles[file] = struct{}{}
		}
		if symbol := strings.TrimSpace(chunk.SymbolName); symbol != "" {
			allowedSymbols[symbol] = struct{}{}
		}
	}
	// 3. 验证 report.RelatedFiles 中的每个文件是否存在于 allowedFiles 中。
	for _, file := range report.RelatedFiles {
		if _, ok := allowedFiles[file]; !ok {
			return fmt.Errorf("%w: related_file %q is not present in context_chunks", ErrLLMInvalidReport, file)
		}
	}
	// 4. 验证 report.RelatedSymbols 中的每个符号是否存在于 allowedSymbols 中。
	for _, symbol := range report.RelatedSymbols {
		if _, ok := allowedSymbols[symbol]; !ok {
			return fmt.Errorf("%w: related_symbol %q is not present in context_chunks", ErrLLMInvalidReport, symbol)
		}
	}
	return nil
}

// extractFencedJSON 从原始字符串中提取 fenced JSON（```json ... ```）的内容。
// 如果找到了 fenced JSON，则返回提取的内容和 true；否则返回空字符串和 false。
func extractFencedJSON(raw string) (string, bool) {
	// 1. 查找第一个 ``` 的位置，如果不存在则返回 false。
	start := strings.Index(raw, "```")
	if start < 0 {
		return "", false
	}

	// 2. 提取 fenced JSON 的内容，并检查是否以 "json" 开头。
	afterStart := raw[start+3:]
	firstLineEnd := strings.IndexByte(afterStart, '\n')
	if firstLineEnd < 0 {
		return "", false
	}

	// 3. 检查 fenced JSON 的标签是否为 "json"，如果不是则返回 false。
	fenceLabel := strings.TrimSpace(afterStart[:firstLineEnd])
	if fenceLabel != "" && !strings.EqualFold(fenceLabel, "json") {
		return "", false
	}

	// 4. 查找 fenced JSON 的结束位置，并返回提取的内容。
	bodyAndRest := afterStart[firstLineEnd+1:]
	end := strings.Index(bodyAndRest, "```")
	if end < 0 {
		return "", false
	}

	// 5. 返回提取的 fenced JSON 内容和 true。
	return strings.TrimSpace(bodyAndRest[:end]), true
}
