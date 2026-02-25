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

	cleanArgs := strings.TrimSpace(args)
	if strings.HasPrefix(cleanArgs, "```json") {
		cleanArgs = cleanArgs[7:]
	} else if strings.HasPrefix(cleanArgs, "```") {
		cleanArgs = cleanArgs[3:]
	}
	if strings.HasSuffix(cleanArgs, "```") {
		cleanArgs = cleanArgs[:len(cleanArgs)-3]
	}

	if err := json.Unmarshal([]byte(cleanArgs), &input); err != nil {
		return "", fmt.Errorf("JSON parsing failed: %w. Ensure you are sending a valid JSON object.", err)
	}

	// Read existing content: check shadow first, fallback to real filesystem
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

	// Parse and apply blocks natively using the centralized fuzzy matcher
	newContent, err := fs.ApplyBlocksToString(string(content), input.Blocks)
	if err != nil {
		return "", err
	}

	// Write the modified content back to the shadow layer
	shadowPath, err := t.shadow.Write(input.Path, []byte(newContent))
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Success: File %s edited and saved to shadow storage at %s", input.Path, shadowPath), nil
}
