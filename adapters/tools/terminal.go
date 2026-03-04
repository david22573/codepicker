package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/david22573/codepicker/domain/errors"
	"github.com/david22573/codepicker/infra/fs"
	"github.com/david22573/codepicker/infra/shell"
	"github.com/david22573/codepicker/infra/validation"
)

type ShellTool struct {
	exec   *shell.Executor
	shadow *fs.ShadowManager
}

func NewShellTool(e *shell.Executor, shadow *fs.ShadowManager) *ShellTool {
	return &ShellTool{
		exec:   e,
		shadow: shadow,
	}
}

func (t *ShellTool) Name() string { return "run_cmd" }
func (t *ShellTool) Description() string {
	return `Execute a shell command.
Input JSON: {"command": "string"}
NOTE: If you have pending edits, this will automatically run in a temporary sandbox containing those edits.`
}

func (t *ShellTool) Execute(ctx context.Context, args string) (string, error) {
	var input struct {
		Command string `json:"command"`
	}

	if err := validation.DecodeStrict(args, &input); err != nil {
		return "", errors.NewValidation("tool.run_cmd", "invalid JSON arguments: "+err.Error())
	}

	if strings.TrimSpace(input.Command) == "" {
		return "", errors.NewValidation("tool.run_cmd", "command cannot be empty")
	}

	// Fix: Bypass strings.Fields error by utilizing a dedicated shell engine string array
	parts := []string{"sh", "-c", input.Command}

	pendingFiles, err := t.shadow.ListShadowFiles()
	if err == nil && len(pendingFiles) > 0 {
		return t.executeInSandbox(ctx, parts, pendingFiles)
	}

	return t.exec.Run(ctx, parts[0], parts[1:]...)
}

func (t *ShellTool) executeInSandbox(ctx context.Context, parts []string, changedFiles []string) (string, error) {
	fmt.Printf("📦 [run_cmd] Detected %d pending changes. Switching to SANDBOX mode for verification.\n", len(changedFiles))

	sandbox, err := fs.NewSandbox(t.shadow.ProjectRoot)
	if err != nil {
		return "", fmt.Errorf("failed to create verification sandbox: %w", err)
	}
	defer sandbox.Cleanup()

	if err := t.shadow.ExportTo(sandbox.SandboxRoot); err != nil {
		return "", fmt.Errorf("failed to sync shadow files to sandbox: %w", err)
	}

	sandboxExec := shell.NewExecutor(
		t.exec.Timeout,
		t.exec.MaxOutput,
		t.exec.DryRun,
		sandbox.SandboxRoot,
	)

	output, err := sandboxExec.Run(ctx, parts[0], parts[1:]...)

	footer := fmt.Sprintf("\n\n--- [Sandbox Execution Notice] ---\nCommand executed in a temporary environment with your %d pending changes applied.\nReal files were NOT modified.", len(changedFiles))

	return output + footer, err
}
