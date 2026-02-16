package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type GitDiffTool struct {
	ProjectRoot string
}

func NewGitDiffTool(root string) *GitDiffTool {
	return &GitDiffTool{ProjectRoot: root}
}

func (t *GitDiffTool) Name() string { return "git_diff" }

func (t *GitDiffTool) Description() string {
	return "Shows the unstaged changes in the working directory. Use this to verify your edits without re-reading the whole file."
}

func (t *GitDiffTool) Execute(ctx context.Context, argsJSON string) (string, error) {
	// argsJSON is ignored as git diff works on the repo state
	cmd := exec.CommandContext(ctx, "git", "diff")
	cmd.Dir = t.ProjectRoot

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git diff failed: %s", string(out))
	}

	output := string(out)
	if strings.TrimSpace(output) == "" {
		return "No changes found (working tree clean).", nil
	}

	return output, nil
}
