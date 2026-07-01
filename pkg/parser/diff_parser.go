package parser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	DiffLineAdded   = "added"
	DiffLineDeleted = "deleted"
	DiffLineContext = "context"

	DiffChangeAdded    = "added"
	DiffChangeDeleted  = "deleted"
	DiffChangeModified = "modified"
)

type DiffFile struct {
	FilePath    string     `json:"file_path"`
	OldPath     string     `json:"old_path"`
	NewPath     string     `json:"new_path"`
	ChangedType string     `json:"changed_type"`
	Hunks       []DiffHunk `json:"hunks"`
}

type DiffHunk struct {
	OldStart int        `json:"old_start"`
	OldCount int        `json:"old_count"`
	NewStart int        `json:"new_start"`
	NewCount int        `json:"new_count"`
	Lines    []DiffLine `json:"lines"`
}

type DiffLine struct {
	Type    string `json:"type"`
	OldLine int    `json:"old_line"`
	NewLine int    `json:"new_line"`
	Content string `json:"content"`
}

var hunkHeaderPattern = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

func ParseDiff(diffText string) ([]DiffFile, error) {
	diffText = strings.TrimPrefix(diffText, "\uFEFF")
	if strings.TrimSpace(diffText) == "" {
		return nil, fmt.Errorf("diff text is empty")
	}

	lines := splitDiffLines(diffText)
	files := make([]DiffFile, 0)
	currentFile := (*DiffFile)(nil)
	currentHunk := (*DiffHunk)(nil)
	oldLine := 0
	newLine := 0
	seenHunk := false

	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git ") {
			if currentFile != nil {
				files = append(files, *currentFile)
			}
			file, err := parseGitDiffLine(line)
			if err != nil {
				return nil, err
			}
			currentFile = file
			currentHunk = nil
			continue
		}

		if currentFile == nil {
			if strings.TrimSpace(line) == "" {
				continue
			}
			return nil, fmt.Errorf("diff file header is required before line: %s", line)
		}

		switch {
		case strings.HasPrefix(line, "--- "):
			currentFile.OldPath = normalizeDiffPath(strings.TrimSpace(strings.TrimPrefix(line, "--- ")))
			updateDiffFileType(currentFile)
		case strings.HasPrefix(line, "+++ "):
			currentFile.NewPath = normalizeDiffPath(strings.TrimSpace(strings.TrimPrefix(line, "+++ ")))
			updateDiffFileType(currentFile)
		case strings.HasPrefix(line, "@@ "):
			hunk, parsedOldLine, parsedNewLine, err := parseHunkHeader(line)
			if err != nil {
				return nil, err
			}
			currentFile.Hunks = append(currentFile.Hunks, hunk)
			currentHunk = &currentFile.Hunks[len(currentFile.Hunks)-1]
			oldLine = parsedOldLine
			newLine = parsedNewLine
			seenHunk = true
		case strings.HasPrefix(line, `\ No newline at end of file`):
			continue
		case currentHunk != nil && strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++ "):
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Type:    DiffLineAdded,
				OldLine: 0,
				NewLine: newLine,
				Content: strings.TrimPrefix(line, "+"),
			})
			newLine++
		case currentHunk != nil && strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "--- "):
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Type:    DiffLineDeleted,
				OldLine: oldLine,
				NewLine: 0,
				Content: strings.TrimPrefix(line, "-"),
			})
			oldLine++
		case currentHunk != nil && strings.HasPrefix(line, " "):
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Type:    DiffLineContext,
				OldLine: oldLine,
				NewLine: newLine,
				Content: strings.TrimPrefix(line, " "),
			})
			oldLine++
			newLine++
		case strings.TrimSpace(line) == "":
			continue
		}
	}

	if currentFile != nil {
		files = append(files, *currentFile)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no diff files found")
	}
	if !seenHunk {
		return nil, fmt.Errorf("no diff hunks found")
	}

	return files, nil
}

func splitDiffLines(diffText string) []string {
	normalized := strings.ReplaceAll(diffText, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	return strings.Split(normalized, "\n")
}

func parseGitDiffLine(line string) (*DiffFile, error) {
	parts := strings.Fields(line)
	if len(parts) < 4 {
		return nil, fmt.Errorf("invalid git diff header: %s", line)
	}

	oldPath := normalizeDiffPath(parts[2])
	newPath := normalizeDiffPath(parts[3])
	file := &DiffFile{
		FilePath:    newPath,
		OldPath:     oldPath,
		NewPath:     newPath,
		ChangedType: DiffChangeModified,
	}
	updateDiffFileType(file)
	return file, nil
}

func parseHunkHeader(line string) (DiffHunk, int, int, error) {
	matches := hunkHeaderPattern.FindStringSubmatch(line)
	if len(matches) == 0 {
		return DiffHunk{}, 0, 0, fmt.Errorf("invalid hunk header: %s", line)
	}

	oldStart, err := strconv.Atoi(matches[1])
	if err != nil {
		return DiffHunk{}, 0, 0, fmt.Errorf("invalid old start in hunk header: %w", err)
	}
	oldCount := parseOptionalCount(matches[2])
	newStart, err := strconv.Atoi(matches[3])
	if err != nil {
		return DiffHunk{}, 0, 0, fmt.Errorf("invalid new start in hunk header: %w", err)
	}
	newCount := parseOptionalCount(matches[4])

	return DiffHunk{
		OldStart: oldStart,
		OldCount: oldCount,
		NewStart: newStart,
		NewCount: newCount,
		Lines:    make([]DiffLine, 0),
	}, oldStart, newStart, nil
}

func parseOptionalCount(value string) int {
	if value == "" {
		return 1
	}
	count, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return count
}

func normalizeDiffPath(path string) string {
	path = strings.Trim(path, `"`)
	if path == "/dev/null" {
		return path
	}
	path = strings.TrimPrefix(path, "a/")
	path = strings.TrimPrefix(path, "b/")
	return path
}

func updateDiffFileType(file *DiffFile) {
	switch {
	case file.OldPath == "/dev/null":
		file.ChangedType = DiffChangeAdded
		file.FilePath = file.NewPath
	case file.NewPath == "/dev/null":
		file.ChangedType = DiffChangeDeleted
		file.FilePath = file.OldPath
	default:
		file.ChangedType = DiffChangeModified
		if file.NewPath != "" {
			file.FilePath = file.NewPath
		} else {
			file.FilePath = file.OldPath
		}
	}
}
