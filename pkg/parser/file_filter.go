package parser

import (
	"path"
	"strings"
)

var ignoredDirs = map[string]struct{}{
	".git":         {},
	"vendor":       {},
	"node_modules": {},
	"tmp":          {},
	"dist":         {},
	".idea":        {},
	".vscode":      {},
}

var allowedExts = map[string]struct{}{
	".go":   {},
	".md":   {},
	".yaml": {},
	".yml":  {},
	".json": {},
	".lua":  {},
	".sql":  {},
	".mod":  {},
	".sum":  {},
}

func ShouldKeepFile(relativePath string) bool {
	cleanPath := normalizeZipPath(relativePath)
	if cleanPath == "." || cleanPath == "" {
		return false
	}

	for _, part := range strings.Split(cleanPath, "/") {
		if _, ok := ignoredDirs[part]; ok {
			return false
		}
	}

	_, ok := allowedExts[strings.ToLower(path.Ext(cleanPath))]
	return ok
}

func FileType(relativePath string) string {
	return strings.TrimPrefix(strings.ToLower(path.Ext(normalizeZipPath(relativePath))), ".")
}

func normalizeZipPath(relativePath string) string {
	return path.Clean(strings.ReplaceAll(relativePath, "\\", "/"))
}
