package shell

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/david22573/codepicker/domain/errors"
)

type Executor struct {
	Timeout   time.Duration
	MaxOutput int
}

func NewExecutor(timeout time.Duration, maxOutput int) *Executor {
	return &Executor{
		Timeout:   timeout,
		MaxOutput: maxOutput,
	}
}

func (e *Executor) Run(ctx context.Context, command string, args ...string) (string, error) {
	// Create a context with timeout if one isn't already set
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, e.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, command, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\nSTDERR:\n" + stderr.String()
	}

	// Truncate if too long
	if len(output) > e.MaxOutput {
		output = output[:e.MaxOutput] + "\n...(truncated)"
	}

	if ctx.Err() == context.DeadlineExceeded {
		return output, errors.NewSystem("shell.Run", "command timed out", ctx.Err())
	}

	if err != nil {
		return output, errors.NewSystem("shell.Run", fmt.Sprintf("command failed: %v", err), err)
	}

	return output, nil
}
