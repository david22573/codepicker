package tools

import (
	"context"
	"fmt"

	"github.com/david22573/codepicker/infra/fs"
	"github.com/david22573/codepicker/infra/validation"
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

	if err := validation.DecodeStrict(args, &input); err != nil {
		return "", err
	}

	if input.Path == "" {
		return "", fmt.Errorf("validation error: missing required field 'path'")
	}

	path, err := t.shadow.Write(input.Path, []byte(input.Content))
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Success: File written to shadow storage at %s", path), nil
}