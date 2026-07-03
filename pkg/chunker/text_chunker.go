package chunker

import (
	"fmt"
	"os"
	pathpkg "path"
	"strings"

	projecthash "pr-guard-agent/pkg/hash"
)

const (
	textChunkSize = 1500
	textOverlap   = 200
)

type TextChunk struct {
	FilePath        string
	SymbolName      string
	SymbolType      string
	StartLine       int
	EndLine         int
	ChunkText       string
	ContentHash     string
	CodeVersionHash string
}

func SplitTextFileToChunks(filePath string, relativePath string, codeVersionHash string) ([]TextChunk, error) {
	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read text file failed: %w", err)
	}

	content := string(contentBytes)
	runes := []rune(content)
	lineIndex := buildLineIndex(runes)
	symbolType := textSymbolType(relativePath)

	if len(runes) <= textChunkSize {
		return []TextChunk{
			buildTextChunk(relativePath, relativePath, symbolType, content, 0, len(runes), lineIndex, codeVersionHash),
		}, nil
	}

	chunks := make([]TextChunk, 0)
	for start, chunkIndex := 0, 1; start < len(runes); chunkIndex++ {
		end := start + textChunkSize
		if end > len(runes) {
			end = len(runes)
		}

		chunkText := string(runes[start:end])
		symbolName := fmt.Sprintf("%s#chunk_%d", relativePath, chunkIndex)
		chunks = append(chunks, buildTextChunk(relativePath, symbolName, symbolType, chunkText, start, end, lineIndex, codeVersionHash))

		if end == len(runes) {
			break
		}

		nextStart := end - textOverlap
		if nextStart <= start {
			nextStart = end
		}
		start = nextStart
	}

	return chunks, nil
}

func buildTextChunk(relativePath string, symbolName string, symbolType string, chunkText string, start int, end int, lineIndex []int, codeVersionHash string) TextChunk {
	startLine := lineNumberAt(lineIndex, start)
	endLine := lineNumberAt(lineIndex, end)
	if endLine < startLine {
		endLine = startLine
	}

	return TextChunk{
		FilePath:        relativePath,
		SymbolName:      symbolName,
		SymbolType:      symbolType,
		StartLine:       startLine,
		EndLine:         endLine,
		ChunkText:       chunkText,
		ContentHash:     projecthash.SHA256String(chunkText),
		CodeVersionHash: codeVersionHash,
	}
}

func buildLineIndex(runes []rune) []int {
	lineIndex := make([]int, len(runes)+1)
	line := 1
	for i, r := range runes {
		lineIndex[i] = line
		if r == '\n' {
			line++
		}
	}
	lineIndex[len(runes)] = line
	return lineIndex
}

func lineNumberAt(lineIndex []int, index int) int {
	if len(lineIndex) == 0 {
		return 1
	}
	if index < 0 {
		index = 0
	}
	if index >= len(lineIndex) {
		index = len(lineIndex) - 1
	}
	if lineIndex[index] < 1 {
		return 1
	}
	return lineIndex[index]
}

func textSymbolType(relativePath string) string {
	ext := strings.ToLower(pathpkg.Ext(strings.ReplaceAll(relativePath, "\\", "/")))
	switch ext {
	case ".md":
		return "markdown"
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	case ".sql":
		return "sql"
	case ".lua":
		return "lua"
	case ".mod", ".sum":
		return "gomod"
	default:
		return "text"
	}
}
