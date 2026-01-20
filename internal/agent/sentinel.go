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

// Command Categories
const (
	ClassReadOnly  = "read-only"
	ClassWrite     = "filesystem-write"
	ClassNetwork   = "network"
	ClassDangerous = "dangerous"
	ClassUnknown   = "unknown"
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

// ClassifyCommand determines the risk profile of a command
func (s *Sentinel) ClassifyCommand(cmdStr string) string {
	parts, err := shlex.Split(cmdStr)
	if err != nil || len(parts) == 0 {
		return ClassUnknown
	}

	binary := parts[0]

	// 1. Check Explicit Dangerous Patterns
	for _, pattern := range dangerousPatterns {
		if matched, _ := regexp.MatchString(pattern, cmdStr); matched {
			return ClassDangerous
		}
	}

	// 2. Network Tools
	if binary == "curl" || binary == "wget" || binary == "git" || binary == "ssh" || binary == "scp" {
		return ClassNetwork
	}

	// 3. Write/Modifying Tools
	if binary == "mv" || binary == "cp" || binary == "rm" || binary == "chmod" || binary == "mkdir" || binary == "touch" {
		return ClassWrite
	}

	// 4. Build Tools (Could be anything, but usually write/net)
	if binary == "go" || binary == "npm" || binary == "make" || binary == "docker" {
		return ClassWrite // conservatively classify as write/complex
	}

	// 5. Read-Only / Safe
	if s.SafeBinaries[binary] {
		// Check for output redirection which turns a safe command into a write
		if strings.Contains(cmdStr, ">") {
			return ClassWrite
		}
		return ClassReadOnly
	}

	return ClassUnknown
}

// CheckCommand performs a deep security scan of the command args
func (s *Sentinel) CheckCommand(cmdStr string) (bool, string, string, []string) {
	classification := s.ClassifyCommand(cmdStr)

	parts, _ := shlex.Split(cmdStr)
	binary := parts[0]
	args := parts[1:]

	if classification == ClassDangerous {
		return true, "Potentially dangerous command pattern detected", binary, args
	}

	// Refined flag checks
	if binary == "find" {
		for _, arg := range args {
			if strings.HasPrefix(arg, "-exec") || strings.HasPrefix(arg, "-delete") || strings.HasPrefix(arg, "-ok") {
				return true, fmt.Sprintf("Forbidden flag '%s' used with find", arg), binary, args
			}
		}
	}

	// System directory protection
	for _, arg := range args {
		if strings.HasPrefix(arg, "/etc") ||
			strings.HasPrefix(arg, "/sys") ||
			strings.HasPrefix(arg, "/proc") {
			return true, "Attempt to access system directory", binary, args
		}
	}

	return false, "", binary, args
}

// BoundedBuffer limits output capture
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
