package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/infra/fs"
	"github.com/david22573/codepicker/infra/git"
	"github.com/david22573/codepicker/infra/validation"
)

type EditFileTool struct {
	projectRoot string
	shadow      *fs.ShadowManager
	gitClient   *git.Client
	llm         agent.LLMClient
	autoCommit  bool
}

func NewEditFileTool(root string, shadow *fs.ShadowManager, gitClient *git.Client, llm agent.LLMClient, autoCommit bool) *EditFileTool {
	return &EditFileTool{
		projectRoot: root,
		shadow:      shadow,
		gitClient:   gitClient,
		llm:         llm,
		autoCommit:  autoCommit,
	}
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
		content, err = fs.SafeReadFile(ctx, realPath)
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

	resultMsg := fmt.Sprintf("Success: File %s edited and saved to shadow storage at %s", input.Path, shadowPath)

	if t.autoCommit && t.gitClient != nil && t.llm != nil {
		realPath := filepath.Join(t.projectRoot, input.Path)
		if err := os.WriteFile(realPath, []byte(newContent), 0644); err != nil {
			return resultMsg + fmt.Sprintf("\n[Auto-commit skipped: failed to write real file: %v]", err), nil
		}

		if err := t.gitClient.StageFiles([]string{input.Path}); err != nil {
			return resultMsg + fmt.Sprintf("\n[Auto-commit skipped: failed to stage file: %v]", err), nil
		}

		prompt := fmt.Sprintf("Write a concise, one-sentence git commit message explaining this change:\n\n%s", input.Blocks)

		// Adjust method name if your agent.LLMClient interface uses something other than Chat
		msg, err := t.llm.Chat(ctx, "You are a senior engineer writing git commit messages. Respond with ONLY the commit message, no markdown formatting, no quotes.", prompt)
		if err != nil {
			msg = "Update " + input.Path
		}

		msg = strings.TrimSpace(msg)
		msg = strings.Trim(msg, "\"`'")
		commitMsg := fmt.Sprintf("[codepicker] %s", msg)

		hash, err := t.gitClient.CommitWithMessage(ctx, commitMsg)
		if err != nil {
			return resultMsg + fmt.Sprintf("\n[Auto-commit skipped: git commit failed: %v]", err), nil
		}

		resultMsg += fmt.Sprintf("\n✅ Auto-committed to real repository (%s): %s", hash, commitMsg)
	}

	return resultMsg, nil
}
