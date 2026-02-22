package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/david22573/codepicker/domain/errors"
	"github.com/david22573/codepicker/infra/fs"
	"github.com/david22573/codepicker/infra/shell"
)

type ShellTool struct {
	exec   *shell.Executor
	shadow *fs.ShadowManager // Injected to check for pending changes
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
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "", errors.NewValidation("tool.run_cmd", "invalid JSON arguments")
	}

	// Basic safety check
	if strings.TrimSpace(input.Command) == "" {
		return "", errors.NewValidation("tool.run_cmd", "command cannot be empty")
	}

	// Parse the command into executable and arguments to avoid bash -c injection
	parts := strings.Fields(input.Command)
	if len(parts) == 0 {
		return "", errors.NewValidation("tool.run_cmd", "malformed command")
	}

	// 1. Check for Pending Shadow Changes
	// If the agent has modified files that aren't committed yet, running 'go test'
	// on the real directory will return results for the OLD code.
	// We must detect this and spin up a sandbox.
	pendingFiles, err := t.shadow.ListShadowFiles()
	if err == nil && len(pendingFiles) > 0 {
		return t.executeInSandbox(ctx, parts, pendingFiles)
	}

	// 2. Fast Path: No changes, run directly in project root
	return t.exec.Run(ctx, parts[0], parts[1:]...)
}

// executeInSandbox spins up a temporary copy of the repo, applies shadow changes, and runs the command.
func (t *ShellTool) executeInSandbox(ctx context.Context, parts []string, changedFiles []string) (string, error) {
	fmt.Printf("📦 [run_cmd] Detected %d pending changes. Switching to SANDBOX mode for verification.\n", len(changedFiles))

	// A. Create Sandbox (copy real files)
	sandbox, err := fs.NewSandbox(t.shadow.ProjectRoot)
	if err != nil {
		return "", fmt.Errorf("failed to create verification sandbox: %w", err)
	}
	defer sandbox.Cleanup()

	// B. Apply Shadow Overlay (overwrite with agent's changes)
	if err := t.shadow.ExportTo(sandbox.SandboxRoot); err != nil {
		return "", fmt.Errorf("failed to sync shadow files to sandbox: %w", err)
	}

	// C. Create a temporary Executor locked to the Sandbox
	// We clone the configuration of the main executor but swap the directory.
	sandboxExec := shell.NewExecutor(
		t.exec.Timeout,
		t.exec.MaxOutput,
		t.exec.DryRun,
		sandbox.SandboxRoot,
	)

	// D. Execute safely without shell interpolation
	output, err := sandboxExec.Run(ctx, parts[0], parts[1:]...)

	// Add a footer to the output so the Agent knows what happened
	footer := fmt.Sprintf("\n\n--- [Sandbox Execution Notice] ---\nCommand executed in a temporary environment with your %d pending changes applied.\nReal files were NOT modified.", len(changedFiles))

	return output + footer, err
}