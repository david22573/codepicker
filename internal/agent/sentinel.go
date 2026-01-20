package agent

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/david22573/codepicker/internal/config"
	"github.com/google/shlex"
)

type Sentinel struct {
	SafeBinaries map[string]bool
	Limits       *config.Limits
}

var dangerousPatterns = []string{
	`curl.*\|.*sh`,
	`wget.*\|.*sh`,
	`eval`,
	`base64.*-d`,
	`> /dev/`,
	`dd if=`,
	`mkfs`,
	`:(){ :|:& };:`,
}

func NewSentinel(limits *config.Limits) *Sentinel {
	return &Sentinel{
		Limits: limits,
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

// BoundedBuffer limits the amount of data capturing from stdout/stderr
type BoundedBuffer struct {
	b     bytes.Buffer
	limit int
}

func (b *BoundedBuffer) Write(p []byte) (n int, err error) {
	if b.b.Len() >= b.limit {
		return len(p), nil
	}
	toWrite := p
	if b.b.Len()+len(p) > b.limit {
		toWrite = p[:b.limit-b.b.Len()]
	}
	_, err = b.b.Write(toWrite)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (b *BoundedBuffer) String() string {
	return b.b.String()
}

func (s *Sentinel) CheckCommand(cmdStr string) (bool, string, string, []string) {
	for _, pattern := range dangerousPatterns {
		if matched, _ := regexp.MatchString(pattern, cmdStr); matched {
			return true, "Potentially dangerous command pattern detected", "", nil
		}
	}

	parts, err := shlex.Split(cmdStr)
	if err != nil || len(parts) == 0 {
		return false, "empty or malformed command", "", nil
	}

	binary := parts[0]
	args := parts[1:]

	if binary == "find" {
		for _, arg := range args {
			if strings.HasPrefix(arg, "-exec") || strings.HasPrefix(arg, "-delete") || strings.HasPrefix(arg, "-ok") {
				return true, fmt.Sprintf("Forbidden flag '%s' used with find", arg), binary, args
			}
		}
	}

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

func (s *Sentinel) Execute(binary string, args []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.Limits.CommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, args...)
	stdout := &BoundedBuffer{limit: s.Limits.MaxCommandOutput}
	stderr := &BoundedBuffer{limit: s.Limits.MaxCommandOutput}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	output := stdout.String() + stderr.String()

	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("command timed out after %v", s.Limits.CommandTimeout)
	}

	if len(output) >= s.Limits.MaxCommandOutput {
		return output + "\n...[TRUNCATED]...", fmt.Errorf("command output truncated")
	}

	if err != nil {
		return output, fmt.Errorf("command failed: %w", err)
	}
	return output, nil
}
