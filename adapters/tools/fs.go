package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/domain/errors"
	"github.com/david22573/codepicker/infra/fs"
)

// --- Write File Tool ---

type WriteFileTool struct {
	shadow *fs.ShadowManager
}

func NewWriteFileTool(s *fs.ShadowManager) *WriteFileTool {
	return &WriteFileTool{shadow: s}
}

func (t *WriteFileTool) Name() string { return "write_file" }
func (t *WriteFileTool) Description() string {
	return `Write content to a file. Input JSON: {"path": "string", "content": "string"}`
}

func (t *WriteFileTool) Execute(ctx context.Context, args string) (string, error) {
	var input struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "", errors.NewValidation("tool.write_file", "invalid JSON arguments")
	}

	if input.Path == "" || input.Content == "" {
		return "", errors.NewValidation("tool.write_file", "path and content are required")
	}

	savedPath, err := t.shadow.Write(input.Path, []byte(input.Content))
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Success. File written to shadow path: %s", savedPath), nil
}

// --- Read File Tool ---

type ReadFileTool struct {
	shadow *fs.ShadowManager
}

func NewReadFileTool(s *fs.ShadowManager) *ReadFileTool {
	return &ReadFileTool{shadow: s}
}

func (t *ReadFileTool) Name() string { return "read_file" }
func (t *ReadFileTool) Description() string {
	return `Read file content. Input JSON: {"path": "string"}`
}

func (t *ReadFileTool) Execute(ctx context.Context, args string) (string, error) {
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "", errors.NewValidation("tool.read_file", "invalid JSON arguments")
	}

	content, err := t.shadow.Read(input.Path)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

// --- List Files Tool ---

type ListFilesTool struct {
	projectRoot string
}

func NewListFilesTool(root string) *ListFilesTool {
	return &ListFilesTool{projectRoot: root}
}

func (t *ListFilesTool) Name() string { return "list_files" }
func (t *ListFilesTool) Description() string {
	return `List files in a directory (recursive, ignores .git). Input JSON: {"path": "string"}`
}

func (t *ListFilesTool) Execute(ctx context.Context, args string) (string, error) {
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		// Default to root if parsing fails or empty
		input.Path = "."
	}

	targetDir := filepath.Join(t.projectRoot, input.Path)
	var files []string

	err := filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip unreadable
		}
		// Ignore hidden folders (like .git, .codepicker)
		if strings.Contains(path, "/.") || strings.Contains(path, "\\.") {
			return nil
		}
		if !info.IsDir() {
			rel, _ := filepath.Rel(t.projectRoot, path)
			files = append(files, rel)
		}
		return nil
	})

	if err != nil {
		return "", errors.NewSystem("tool.list_files", "walk failed", err)
	}

	if len(files) > 100 {
		return fmt.Sprintf("Found %d files. First 100: %v...", len(files), files[:100]), nil
	}
	return strings.Join(files, "\n"), nil
}
