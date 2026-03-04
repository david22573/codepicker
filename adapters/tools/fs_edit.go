package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/david22573/codepicker/infra/fs"
	"github.com/david22573/codepicker/infra/validation"
)

type EditFileTool struct {
	projectRoot string
	shadow      *fs.ShadowManager
}

func NewEditFileTool(root string, shadow *fs.ShadowManager) *EditFileTool {
	return &EditFileTool{projectRoot: root, shadow: shadow}
}

func (t *EditFileTool) Name() string { return "edit_file" }
func (t *EditFileTool) Description() string {
	return `Edit an existing file using SEARCH/REPLACE blocks.
Input: JSON with "path" and "blocks".
The "blocks" string must use this exact format:
<<<<
exact original code lines here
====
new replacement code lines here
>>>>`
}

func (t *EditFileTool) Execute(ctx context.Context, args string) (string, error) {
	var input struct {
		Path   string `json:"path"`
		Blocks string `json:"blocks"`
	}

	if err := validation.DecodeStrict(args, &input); err != nil {
		return "", err
	}

	if input.Path == "" || input.Blocks == "" {
		return "", fmt.Errorf("validation error: missing required fields 'path' or 'blocks'")
	}

	var content []byte
	var err error
	content, err = t.shadow.Read(input.Path)
	if err != nil {
		realPath := filepath.Join(t.projectRoot, input.Path)
		content, err = os.ReadFile(realPath)
		if err != nil {
			return "", fmt.Errorf("failed to read file '%s' for editing: %w", input.Path, err)
		}
	}

	newContent, err := fs.ApplyBlocksToString(string(content), input.Blocks)
	if err != nil {
		return "", err
	}

	shadowPath, err := t.shadow.Write(input.Path, []byte(newContent))
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Success: File %s edited and saved to shadow storage at %s", input.Path, shadowPath), nil
}
