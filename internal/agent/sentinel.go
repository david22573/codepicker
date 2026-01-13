package agent

import (
	"fmt"
	"os/exec"
	"strings"

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

// CheckCommand analyzes the command string and determines if it requires approval.
// It returns (needsApproval bool, reason string, binary string, args []string).
func (s *Sentinel) CheckCommand(cmdStr string) (bool, string, string, []string) {
	// 1. Use shlex to parse the command string properly (handling quotes)
	parts, err := shlex.Split(cmdStr)
	if err != nil || len(parts) == 0 {
		return false, "empty or malformed command", "", nil
	}

	binary := parts[0]
	args := parts[1:]

	// 2. Strict Whitelist Check
	if s.SafeBinaries[binary] {
		// Scan args for suspicious shell operators just in case
		for _, arg := range args {
			if strings.ContainsAny(arg, "&|;`$") {
				return true, "Suspicious shell characters detected in arguments", binary, args
			}
		}
		return false, "", binary, args
	}

	// 3. Dangerous Binaries
	if binary == "rm" || binary == "mv" || binary == "cp" || binary == "chmod" {
		return true, fmt.Sprintf("File system modification detected: %s", binary), binary, args
	}

	// 4. External Tools (Network/Build)
	if binary == "go" || binary == "npm" || binary == "git" || binary == "curl" || binary == "wget" {
		return true, fmt.Sprintf("External tool execution: %s", binary), binary, args
	}

	return true, fmt.Sprintf("Unrecognized binary: %s", binary), binary, args
}

// Execute runs the command directly without a shell wrapper.
// This kills the class of attacks where an LLM chains commands (e.g. "ls && rm -rf /").
func (s *Sentinel) Execute(binary string, args []string) (string, error) {
	cmd := exec.Command(binary, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("command failed: %w", err)
	}
	return string(output), nil
}
