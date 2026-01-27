package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/domain/errors"
)

type SearchTool struct {
	projectRoot string
}

func NewSearchTool(root string) *SearchTool {
	return &SearchTool{projectRoot: root}
}

func (t *SearchTool) Name() string { return "search_code" }
func (t *SearchTool) Description() string {
	return `Search for a string in non-binary files. Input JSON: {"query": "string", "path": "string"}`
}

func (t *SearchTool) Execute(ctx context.Context, args string) (string, error) {
	var input struct {
		Query string `json:"query"`
		Path  string `json:"path"`
	}
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "", errors.NewValidation("tool.search_code", "invalid JSON arguments")
	}

	if input.Query == "" {
		return "", errors.NewValidation("tool.search_code", "query cannot be empty")
	}
	if input.Path == "" {
		input.Path = "."
	}

	targetDir := filepath.Join(t.projectRoot, input.Path)
	var results strings.Builder
	matchCount := 0

	err := filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip unreadable
		}
		if info.IsDir() {
			// Skip hidden dirs
			if strings.HasPrefix(info.Name(), ".") && info.Name() != "." {
				return filepath.SkipDir
			}
			return nil
		}
		// Skip hidden files
		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		// Simple binary check (skip known binary extensions or huge files)
		ext := strings.ToLower(filepath.Ext(path))
		if isBinaryExt(ext) {
			return nil
		}

		// Read file
		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		lineNum := 1

		relPath, _ := filepath.Rel(t.projectRoot, path)

		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, input.Query) {
				results.WriteString(fmt.Sprintf("%s:%d: %s\n", relPath, lineNum, strings.TrimSpace(line)))
				matchCount++
				if matchCount > 100 {
					results.WriteString("... (limit reached)\n")
					return filepath.SkipDir // Stop searching to save token limit
				}
			}
			lineNum++
		}
		return nil
	})

	if err != nil {
		return "", errors.NewSystem("tool.search_code", "walk failed", err)
	}

	if matchCount == 0 {
		return "No matches found.", nil
	}
	return results.String(), nil
}

func isBinaryExt(ext string) bool {
	switch ext {
	case ".exe", ".dll", ".so", ".dylib", ".bin", ".png", ".jpg", ".jpeg", ".gif", ".ico", ".pdf", ".zip", ".tar", ".gz":
		return true
	default:
		return false
	}
}
