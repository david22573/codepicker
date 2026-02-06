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

// --- WriteFileTool ---

// WriteFileTool allows the agent to write content to the shadow filesystem
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

	// Writes to the shadow directory to prevent direct unverified modification
	savedPath, err := t.shadow.Write(input.Path, []byte(input.Content))
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Success. File written to shadow path: %s", savedPath), nil
}

// --- ReadFileTool ---

// ReadFileTool allows the agent to read content, prioritizing the shadow filesystem
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

	// Reads from shadow if it exists, otherwise falls back to real file
	content, err := t.shadow.Read(input.Path)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

// --- ListFilesTool ---

// ListFilesTool provides a recursive directory listing
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
	// Default to project root if no path is provided
	if err := json.Unmarshal([]byte(args), &input); err != nil || input.Path == "" {
		input.Path = "."
	}

	targetDir := filepath.Join(t.projectRoot, input.Path)
	var files []string

	// Walk the directory and capture all non-hidden files
	err := filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
		// FIX: We now propagate the error so the agent knows if a directory is inaccessible
		if err != nil {
			return fmt.Errorf("access error at %s: %v", path, err)
		}

		// Skip hidden files and directories like .git
		if strings.Contains(path, "/.") || strings.Contains(path, "\\.") {
			if info.IsDir() && info.Name() != "." {
				return filepath.SkipDir
			}
			return nil
		}

		if !info.IsDir() {
			rel, _ := filepath.Rel(t.projectRoot, path)
			files = append(files, rel)
		}
		return nil
	})

	if err != nil {
		return "", errors.NewSystem("tool.list_files", "failed to walk directory", err)
	}

	if len(files) == 0 {
		return "No files found.", nil
	}

	// Truncate output for very large projects to avoid token overflow
	if len(files) > 100 {
		return fmt.Sprintf("Found %d files. Showing first 100:\n%s\n... (truncated)", len(files), strings.Join(files[:100], "\n")), nil
	}

	return strings.Join(files, "\n"), nil
}
