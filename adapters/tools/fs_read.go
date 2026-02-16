package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/infra/fs"
)

// --- ReadFileTool ---

type ReadFileTool struct {
	ProjectRoot string
	Shadow      *fs.ShadowManager // FIX: Inject ShadowManager
}

func NewReadFileTool(root string, shadow *fs.ShadowManager) *ReadFileTool {
	return &ReadFileTool{
		ProjectRoot: root,
		Shadow:      shadow,
	}
}

func (t *ReadFileTool) Name() string {
	return "read_file"
}

func (t *ReadFileTool) Description() string {
	return `Read the contents of a file.
Input: JSON with "path" (e.g., {"path": "main.go"}).`
}

func (t *ReadFileTool) Execute(ctx context.Context, args string) (string, error) {
	cleanPath := strings.TrimSpace(args)
	cleanPath = strings.TrimPrefix(cleanPath, `{"path":`)
	cleanPath = strings.TrimPrefix(cleanPath, `{"path": `)
	cleanPath = strings.TrimSuffix(cleanPath, `}`)
	cleanPath = strings.ReplaceAll(cleanPath, `"`, "")
	cleanPath = strings.TrimSpace(cleanPath)

	// FIX: Check Shadow Layer first!
	// This allows the agent to see changes it just made but hasn't committed yet.
	if content, err := t.Shadow.Read(cleanPath); err == nil {
		return string(content), nil
	}

	fullPath := filepath.Join(t.ProjectRoot, cleanPath)

	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file '%s': %w", cleanPath, err)
	}
	return string(content), nil
}

// --- ListDirTool ---

type ListDirTool struct {
	ProjectRoot string
}

func NewListDirTool(root string) *ListDirTool {
	return &ListDirTool{ProjectRoot: root}
}

func (t *ListDirTool) Name() string {
	return "list_dir"
}

func (t *ListDirTool) Description() string {
	return `List files in a directory.
Input: Relative path string (e.g., "./cmd").`
}

func (t *ListDirTool) Execute(ctx context.Context, args string) (string, error) {
	cleanPath := strings.TrimSpace(strings.ReplaceAll(args, `"`, ""))
	fullPath := filepath.Join(t.ProjectRoot, cleanPath)

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to list directory: %w", err)
	}

	var results []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		results = append(results, name)
	}

	if len(results) == 0 {
		return "(empty directory)", nil
	}
	return strings.Join(results, "\n"), nil
}
