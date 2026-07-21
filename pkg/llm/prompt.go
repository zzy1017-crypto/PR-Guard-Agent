package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type RiskPromptInput struct {
	DiffText      string
	ContextChunks []ContextChunk
}

type ContextChunk struct {
	ChunkID    uint    `json:"chunk_id"`
	FilePath   string  `json:"file_path"`
	SymbolName string  `json:"symbol_name"`
	SymbolType string  `json:"symbol_type"`
	StartLine  int     `json:"start_line"`
	EndLine    int     `json:"end_line"`
	Score      float32 `json:"score"`
	ChunkText  string  `json:"chunk_text"`
}

// BuildRiskAnalysisPrompt 构建用于风险分析的提示字符串，包含 diff_text 和 context_chunks。
// 如果 diff_text 为空，将返回错误。
// context_chunks 将被序列化为 JSON 并包含在提示中。
// 提示中还包含允许的 related_files 和 related_symbols 列表，以及 JSON schema。
func BuildRiskAnalysisPrompt(input RiskPromptInput) (string, error) {

	// 1. 检查 diff_text 是否为空，如果为空则返回错误。
	diffText := strings.TrimSpace(input.DiffText)
	if diffText == "" {
		return "", errors.New("diff_text is empty")
	}

	// 2. 将 context_chunks 序列化为 JSON，如果序列化失败则返回错误。
	contextJSON, err := json.MarshalIndent(input.ContextChunks, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal context_chunks failed: %w", err)
	}

	// 3. 提取 context_chunks 中的唯一文件路径和符号名称，作为允许的 related_files 和 related_symbols。
	allowedFiles := uniqueChunkValues(input.ContextChunks, func(chunk ContextChunk) string {
		return chunk.FilePath
	})
	allowedSymbols := uniqueChunkValues(input.ContextChunks, func(chunk ContextChunk) string {
		return chunk.SymbolName
	})

	// 4. 构建提示字符串，包含角色、规则、允许的文件和符号列表、JSON schema、diff_text 和 context_chunks。
	var b strings.Builder
	b.WriteString("Role: 你是后端代码变更风险分析助手。\n\n")
	b.WriteString("Rules:\n")
	b.WriteString("1. Analyze only the given diff and context_chunks.\n")
	b.WriteString("2. Do not invent files, functions, methods, interfaces, services, modules, or external facts.\n")
	b.WriteString("3. related_files must be selected only from context_chunks.file_path.\n")
	b.WriteString("4. related_symbols must be selected only from context_chunks.symbol_name.\n")
	b.WriteString("5. If context is insufficient, say so in possible_risks or summary.\n")
	b.WriteString("6. Return JSON only. Do not return Markdown. Do not wrap the answer in a ```json code block.\n")
	b.WriteString("7. The JSON object must contain exactly these top-level fields: risk_level, summary, affected_modules, possible_risks, suggested_tests, related_files, related_symbols, confidence.\n")
	b.WriteString("8. risk_level must be one of low, medium, high. confidence must be a number from 0 to 1.\n\n")
	b.WriteString("Allowed related_files:\n")
	b.WriteString(formatStringList(allowedFiles))
	b.WriteString("\n\nAllowed related_symbols:\n")
	b.WriteString(formatStringList(allowedSymbols))
	b.WriteString("\n\nRequired JSON schema:\n")
	b.WriteString(`{"risk_level":"low|medium|high","summary":"string","affected_modules":["string"],"possible_risks":["string"],"suggested_tests":["string"],"related_files":["string"],"related_symbols":["string"],"confidence":0.0}`)
	b.WriteString("\n\nDiff:\n")
	b.WriteString(diffText)
	b.WriteString("\n\ncontext_chunks:\n")
	b.Write(contextJSON)
	if len(input.ContextChunks) == 0 {
		b.WriteString("\n\nContext note: context_chunks is empty; explicitly mention that available context is insufficient.")
	}

	return b.String(), nil
}

// uniqueChunkValues 从 context_chunks 中提取唯一的字符串值，使用提供的 pick 函数选择每个 chunk 的值。
func uniqueChunkValues(chunks []ContextChunk, pick func(ContextChunk) string) []string {

	// 1. 初始化一个空的字符串切片和一个用于跟踪已见值的 map。
	values := make([]string, 0, len(chunks))
	seen := make(map[string]struct{}, len(chunks))
	// 2. 遍历每个 chunk，使用 pick 函数提取值，并进行去重和空值过滤。
	for _, chunk := range chunks {
		value := strings.TrimSpace(pick(chunk))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	return values
}

// formatStringList 将字符串切片格式化为带有每个值前缀的列表字符串，如果切片为空，则返回 "- none\n"。
func formatStringList(values []string) string {
	if len(values) == 0 {
		return "- none\n"
	}

	// 1. 使用 strings.Builder 构建格式化的字符串列表，每个值前面加上 "- " 前缀，并在每个值后面添加换行符。
	var b strings.Builder
	for _, value := range values {
		b.WriteString("- ")
		b.WriteString(value)
		b.WriteString("\n")
	}
	return b.String()
}
