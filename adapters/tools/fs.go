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

// WriteFileTool allows the agent to write content to the shadow filesystem.
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

	// Safety Check: Check size of content before writing
	if len(input.Content) > fs.MaxFileSize {
		return "", fmt.Errorf("content exceeds maximum file size limit")
	}

	savedPath, err := t.shadow.Write(input.Path, []byte(input.Content))
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Success. File written to shadow path: %s", savedPath), nil
}

// ReadFileTool allows the agent to read content, prioritizing shadow with safety limits.
type ReadFileTool struct {
	shadow *fs.ShadowManager
}

func NewReadFileTool(s *fs.ShadowManager) *ReadFileTool {
	return &ReadFileTool{shadow: s}
}

func (t *ReadFileTool) Name() string { return "read_file" }

func (t *ReadFileTool) Description() string {
	return `Read file content safely. Input JSON: {"path": "string"}`
}

func (t *ReadFileTool) Execute(ctx context.Context, args string) (string, error) {
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "", errors.NewValidation("tool.read_file", "invalid JSON arguments")
	}

	// Use the New SafeReadFile logic from infra/fs/safety.go
	// We check shadow first, then fallback to SafeReadFile
	shadowPath := filepath.Join(t.shadow.ProjectRoot, ".codepicker/shadow", input.Path)
	if _, err := os.Stat(shadowPath); err == nil {
		content, err := fs.SafeReadFile(ctx, shadowPath)
		if err != nil {
			return "", err
		}
		return string(content), nil
	}

	realPath := filepath.Join(t.shadow.ProjectRoot, input.Path)
	content, err := fs.SafeReadFile(ctx, realPath)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

// ListFilesTool provides a recursive directory listing with a hard item limit.
type ListFilesTool struct {
	projectRoot string
}

func NewListFilesTool(root string) *ListFilesTool {
	return &ListFilesTool{projectRoot: root}
}

func (t *ListFilesTool) Name() string { return "list_files" }

func (t *ListFilesTool) Description() string {
	return `List files in a directory. Input JSON: {"path": "string"}`
}

func (t *ListFilesTool) Execute(ctx context.Context, args string) (string, error) {
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(args), &input); err != nil || input.Path == "" {
		input.Path = "."
	}

	targetDir := filepath.Join(t.projectRoot, input.Path)
	var files []string
	const maxFiles = 1000 // Acceptance Criteria

	err := filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if len(files) >= maxFiles {
			return filepath.SkipDir
		}
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

	result := strings.Join(files, "\n")
	if len(files) >= maxFiles {
		result += "\n... (truncated: reached 1000 file limit)"
	}

	return result, nil
}
