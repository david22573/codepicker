package agent

import (
	"fmt"
	"os/exec"
	"strings"
)

type Sentinel struct {
	// Whitelist of binaries that are always safe to run read-only
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
			"mkdir": true, // Usually safe in context of creating new project dirs
		},
	}
}

// CheckCommand analyzes a command string and determines if it requires human approval.
// Returns (needsApproval bool, reason string)
func (s *Sentinel) CheckCommand(cmdStr string) (bool, string) {
	parts := strings.Fields(cmdStr)
	if len(parts) == 0 {
		return false, "empty command"
	}

	binary := parts[0]

	// 1. Check Whitelist
	if s.SafeBinaries[binary] {
		return false, ""
	}

	// 2. Specific flagging for dangerous operations
	if binary == "rm" || binary == "mv" {
		return true, fmt.Sprintf("File system modification detected: %s", binary)
	}

	// 3. Network/Package managers usually need approval
	if binary == "go" || binary == "npm" || binary == "git" || binary == "curl" || binary == "wget" {
		// We might refine this later (e.g., 'go list' is safe, 'go run' is not)
		return true, fmt.Sprintf("External tool execution: %s", binary)
	}

	// Default to cautious
	return true, fmt.Sprintf("Unrecognized binary: %s", binary)
}

func (s *Sentinel) Execute(cmdStr string) (string, error) {
	// NOTE: This basic implementation assumes Linux/Termux shell (sh/bash)
	cmd := exec.Command("sh", "-c", cmdStr)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("command failed: %w", err)
	}
	return string(output), nil
}
