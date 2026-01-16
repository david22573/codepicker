package agent

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/google/shlex"
)

type Sentinel struct {
	SafeBinaries map[string]bool
}

func NewSentinel() *Sentinel {
	return &Sentinel{
		SafeBinaries: map[string]bool{
			"ls":    true,
			"cat":   true,
			"grep":  true,
			"find":  true,
			"pwd":   true,
			"echo":  true,
			"mkdir": true,
		},
	}
}

func (s *Sentinel) CheckCommand(cmdStr string) (bool, string, string, []string) {

	parts, err := shlex.Split(cmdStr)
	if err != nil || len(parts) == 0 {
		return false, "empty or malformed command", "", nil
	}

	binary := parts[0]
	args := parts[1:]

	// SECURITY FIX: Block attempts to read sensitive system directories
	if binary == "cat" || binary == "grep" || binary == "find" {
		for _, arg := range args {
			if strings.HasPrefix(arg, "/etc") ||
				strings.HasPrefix(arg, "/sys") ||
				strings.HasPrefix(arg, "/proc") ||
				strings.HasPrefix(arg, "/var") {
				return true, "Attempt to read system directory", binary, args
			}
		}
	}

	if s.SafeBinaries[binary] {
		for _, arg := range args {
			if strings.ContainsAny(arg, "&|;`$") {
				return true, "Suspicious shell characters detected in arguments", binary, args
			}
		}
		return false, "", binary, args
	}

	if binary == "rm" || binary == "mv" || binary == "cp" || binary == "chmod" {
		return true, fmt.Sprintf("File system modification detected: %s", binary), binary, args
	}

	if binary == "go" || binary == "npm" || binary == "git" || binary == "curl" || binary == "wget" {
		return true, fmt.Sprintf("External tool execution: %s", binary), binary, args
	}

	return true, fmt.Sprintf("Unrecognized binary: %s", binary), binary, args
}

// SECURITY FIX: Limit output size to prevent OOM/DoS
const MaxCommandOutput = 1024 * 100 // 100KB max

// BoundedBuffer implements io.Writer but caps the stored data size.
type BoundedBuffer struct {
	b     bytes.Buffer
	limit int
}

func (b *BoundedBuffer) Write(p []byte) (n int, err error) {
	if b.b.Len() >= b.limit {
		return len(p), nil // Silent drop to satisfy io.Writer contract
	}
	toWrite := p
	if b.b.Len()+len(p) > b.limit {
		toWrite = p[:b.limit-b.b.Len()]
	}
	_, err = b.b.Write(toWrite)
	if err != nil {
		return 0, err
	}
	// Always return len(p) so exec.Command thinks the write succeeded
	return len(p), nil
}

func (b *BoundedBuffer) String() string {
	return b.b.String()
}

func (s *Sentinel) Execute(binary string, args []string) (string, error) {
	// SECURITY FIX: Enforce 10 second timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, args...)

	// SECURITY FIX: Use BoundedBuffer to prevent unbounded memory usage
	stdout := &BoundedBuffer{limit: MaxCommandOutput}
	stderr := &BoundedBuffer{limit: MaxCommandOutput}

	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	output := stdout.String() + stderr.String()

	// Check for timeout specifically
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("command timed out after 10 seconds")
	}

	if len(output) >= MaxCommandOutput {
		return output + "\n...[TRUNCATED]...", fmt.Errorf("command output truncated (exceeded %d bytes)", MaxCommandOutput)
	}

	if err != nil {
		return output, fmt.Errorf("command failed: %w", err)
	}
	return output, nil
}
