package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/david22573/codepicker/infra/fs"
)

type WriteFileTool struct {
	shadow *fs.ShadowManager
}

func NewWriteFileTool(shadow *fs.ShadowManager) *WriteFileTool {
	return &WriteFileTool{shadow: shadow}
}

func (t *WriteFileTool) Name() string {
	return "write_file"
}

func (t *WriteFileTool) Description() string {
	return `Write a file to the filesystem.
Input: JSON with "path" and "content".
Example: {"path": "main.go", "content": "package main..."}`
}

func (t *WriteFileTool) Execute(ctx context.Context, args string) (string, error) {
	var input struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}

	// Sanitize input to remove Markdown blocks
	cleanArgs := t.cleanJSON(args)

	if err := json.Unmarshal([]byte(cleanArgs), &input); err != nil {
		return "", fmt.Errorf("JSON parsing failed (ensure content newlines are escaped): %w", err)
	}

	path, err := t.shadow.Write(input.Path, []byte(input.Content))
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Success: File written to shadow storage at %s", path), nil
}

func (t *WriteFileTool) cleanJSON(input string) string {
	input = strings.TrimSpace(input)
	if strings.HasPrefix(input, "```json") {
		input = input[7:]
	} else if strings.HasPrefix(input, "```") {
		input = input[3:]
	}
	if strings.HasSuffix(input, "```") {
		input = input[:len(input)-3]
	}
	return strings.TrimSpace(input)
}
