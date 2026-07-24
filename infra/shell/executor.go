package shell

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/david22573/codepicker/domain/errors"
	"github.com/david22573/codepicker/domain/task"
)

// Executor handles executing shell commands with timeouts and safety checks.
type Executor struct {
	Timeout       time.Duration
	MaxOutput     int
	DryRun        bool
	RestrictedDir string // The Jailed Directory
}

// NewExecutor creates a new shell executor locked to a specific directory.
func NewExecutor(timeout time.Duration, maxOutput int, dryRun bool, rootDir string) *Executor {
	abs, _ := filepath.Abs(rootDir)
	return &Executor{
		Timeout:       timeout,
		MaxOutput:     maxOutput,
		DryRun:        dryRun,
		RestrictedDir: abs,
	}
}

// safeEnv strips context from the host OS, maintaining only essential routing variables.
func (e *Executor) safeEnv() []string {
	allowed := []string{"PATH", "HOME", "USER", "GOPATH", "GOROOT", "GOCACHE"}
	var env []string
	for _, k := range allowed {
		if v := os.Getenv(k); v != "" {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
	}
	return env
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
		if strings.Contains(arg, "-C ") || arg == "-C" {
			return "", errors.NewPolicy("shell.Run", "forbidden flag detected: -C (directory change)")
		}
		if strings.Contains(arg, "--work-tree") {
			return "", errors.NewPolicy("shell.Run", "forbidden flag detected: --work-tree")
		}
		if strings.Contains(arg, "--git-dir") {
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

	// 4. Security: Enforce Working Directory & Strip Environment
	cmd.Dir = e.RestrictedDir
	cmd.Env = e.safeEnv()

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

// RunCommandCheck executes a command and returns a structured CheckResult.
func (e *Executor) RunCommandCheck(ctx context.Context, name, command, workingDir string) task.CheckResult {
	start := time.Now()
	res := task.CheckResult{
		Name:    name,
		Command: command,
		Status:  task.CheckFail,
	}

	if e.DryRun {
		res.Status = task.CheckPass
		res.Stdout = fmt.Sprintf("[DRY-RUN] Would execute: %s in %s", command, workingDir)
		return res
	}

	parts := strings.Fields(command)
	if len(parts) == 0 {
		res.Error = "empty command"
		return res
	}

	// Safety: use a separate context with timeout if not provided
	cmdCtx := ctx
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		cmdCtx, cancel = context.WithTimeout(ctx, e.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(cmdCtx, parts[0], parts[1:]...)
	cmd.Dir = workingDir
	cmd.Env = e.safeEnv()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	res.DurationMS = time.Since(start).Milliseconds()
	res.Stdout = stdout.String()
	res.Stderr = stderr.String()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			res.ExitCode = exitErr.ExitCode()
		} else {
			res.ExitCode = -1
		}
		res.Error = err.Error()
		res.Status = task.CheckFail
	} else {
		res.Status = task.CheckPass
		res.ExitCode = 0
	}

	return res
}
