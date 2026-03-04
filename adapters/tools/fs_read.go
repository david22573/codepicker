package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/infra/fs"
	"github.com/david22573/codepicker/infra/validation"
)

// --- ReadFileTool ---

type ReadFileTool struct {
	ProjectRoot string
	Shadow      *fs.ShadowManager
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
	var input struct {
		Path string `json:"path"`
	}

	if err := validation.DecodeStrict(args, &input); err != nil {
		return "", err
	}

	cleanPath := filepath.Clean(strings.TrimSpace(input.Path))
	if cleanPath == "" || cleanPath == "." {
		return "", fmt.Errorf("validation error: missing or invalid required field 'path'")
	}

	var content []byte
	var err error

	// 1. Check Shadow Layer first (Agent's pending changes)
	content, err = t.Shadow.Read(cleanPath)
	if err != nil {
		// 2. Read from Real Filesystem (Safe Mode)
		fullPath := filepath.Join(t.ProjectRoot, cleanPath)
		content, err = fs.SafeReadFile(ctx, fullPath)
		if err != nil {
			return "", fmt.Errorf("failed to read file '%s': %w", cleanPath, err)
		}
	}

	// Add line numbers for spatial anchoring
	lines := strings.Split(string(content), "\n")
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("--- Contents of %s ---\n", cleanPath))
	builder.WriteString("IMPORTANT: Line numbers are provided for reference only.\n")
	builder.WriteString("DO NOT include the line numbers in your edit_file SEARCH/REPLACE blocks!\n\n")

	for i, line := range lines {
		builder.WriteString(fmt.Sprintf("%4d | %s\n", i+1, line))
	}

	return builder.String(), nil
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
Input: Relative path string (e.g., {"path": "./cmd"}).`
}

func (t *ListDirTool) Execute(ctx context.Context, args string) (string, error) {
	var input struct {
		Path string `json:"path"`
	}

	if err := validation.DecodeStrict(args, &input); err != nil {
		return "", err
	}

	cleanPath := filepath.Clean(strings.TrimSpace(input.Path))
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
