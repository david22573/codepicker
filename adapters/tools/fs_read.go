package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/infra/fs"
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

	cleanPath := ""
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		// Fallback: treat entire string as path (for backward compat if LLM forgets JSON)
		cleanPath = strings.Trim(strings.TrimSpace(args), `"' `)
	} else {
		cleanPath = filepath.Clean(strings.TrimSpace(input.Path))
	}

	if cleanPath == "" {
		return "", fmt.Errorf("path cannot be empty")
	}

	// 1. Check Shadow Layer first (Agent's pending changes)
	if content, err := t.Shadow.Read(cleanPath); err == nil {
		return string(content), nil
	}

	// 2. Read from Real Filesystem (Safe Mode)
	fullPath := filepath.Join(t.ProjectRoot, cleanPath)

	// Use SafeReadFile to enforce size limits and binary detection
	content, err := fs.SafeReadFile(ctx, fullPath)
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
Input: Relative path string (e.g., {"path": "./cmd"}).`
}

func (t *ListDirTool) Execute(ctx context.Context, args string) (string, error) {
	var input struct {
		Path string `json:"path"`
	}

	cleanPath := ""
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		// Fallback handling
		cleanPath = strings.TrimSpace(strings.ReplaceAll(args, `"`, ""))
	} else {
		cleanPath = filepath.Clean(strings.TrimSpace(input.Path))
	}

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