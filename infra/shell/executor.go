package shell

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/david22573/codepicker/domain/errors"
)

// Executor handles executing shell commands with timeouts and safety checks.
type Executor struct {
	Timeout       time.Duration
	MaxOutput     int
	DryRun        bool
	RestrictedDir string // The Jailed Directory
}

// NewExecutor creates a new shell executor locked to a specific directory.
// UPDATED: Now requires rootDir to enforce workspace isolation.
func NewExecutor(timeout time.Duration, maxOutput int, dryRun bool, rootDir string) *Executor {
	abs, _ := filepath.Abs(rootDir)
	return &Executor{
		Timeout:       timeout,
		MaxOutput:     maxOutput,
		DryRun:        dryRun,
		RestrictedDir: abs,
	}
}

// Run executes a command and returns its output (stdout + stderr).
// It respects context cancellation, configured timeout, and directory constraints.
func (e *Executor) Run(ctx context.Context, command string, args ...string) (string, error) {
	// 1. Safety Check: Dry Run
	if e.DryRun {
		return fmt.Sprintf("[DRY-RUN] Would execute: %s %v in %s", command, args, e.RestrictedDir), nil
	}

	// 2. Security: Validate arguments for escape attempts
	for _, arg := range args {
		// Block flags that tools use to change directory context despite cmd.Dir
		if strings.Contains(arg, "-C ") || arg == "-C" { // Common in git, tar, make
			return "", errors.NewPolicy("shell.Run", "forbidden flag detected: -C (directory change)")
		}
		if strings.Contains(arg, "--work-tree") { // Git specific escape
			return "", errors.NewPolicy("shell.Run", "forbidden flag detected: --work-tree")
		}
		if strings.Contains(arg, "--git-dir") { // Git specific escape
			return "", errors.NewPolicy("shell.Run", "forbidden flag detected: --git-dir")
		}
	}

	// 3. Create context with timeout if not present
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, e.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, command, args...)

	// 4. Security: Enforce Working Directory
	// This ensures "ls" lists the project, not the system root.
	cmd.Dir = e.RestrictedDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// 5. Execute
	err := cmd.Run()

	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\nSTDERR:\n" + stderr.String()
	}

	// 6. Truncate output
	if len(output) > e.MaxOutput {
		output = output[:e.MaxOutput] + "\n...(truncated)"
	}

	// 7. Error Handling
	if ctx.Err() == context.DeadlineExceeded {
		return output, errors.NewSystem("shell.Run", "command timed out", ctx.Err())
	}

	if err != nil {
		return output, errors.NewSystem("shell.Run", fmt.Sprintf("command failed: %v", err), err)
	}

	return output, nil
}
