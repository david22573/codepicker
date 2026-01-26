package tools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/david22573/codepicker/domain/errors"
	"github.com/david22573/codepicker/infra/shell"
)

type ShellTool struct {
	exec *shell.Executor
}

func NewShellTool(e *shell.Executor) *ShellTool {
	return &ShellTool{exec: e}
}

func (t *ShellTool) Name() string { return "run_cmd" }
func (t *ShellTool) Description() string {
	return `Execute a shell command. Input JSON: {"command": "string"}`
}

func (t *ShellTool) Execute(ctx context.Context, args string) (string, error) {
	var input struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "", errors.NewValidation("tool.run_cmd", "invalid JSON arguments")
	}

	// Basic safety check is handled by Policy, but we can have a fallback here
	if strings.TrimSpace(input.Command) == "" {
		return "", errors.NewValidation("tool.run_cmd", "command cannot be empty")
	}

	// We pass the command to bash to allow pipes and simple logic
	return t.exec.Run(ctx, "bash", "-c", input.Command)
}
