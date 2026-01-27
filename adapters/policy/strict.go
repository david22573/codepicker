package policy

import (
	"strings"
)

type StrictPolicy struct {
	blockedCmds []string
	readOnly    bool
	ciMode      bool
}

// NewStrictPolicy creates a policy instance.
func NewStrictPolicy(readOnly, ciMode bool) *StrictPolicy {
	return &StrictPolicy{
		readOnly: readOnly,
		ciMode:   ciMode,
		blockedCmds: []string{
			"rm -rf /", "rm -rf ~", "sudo", "su ", ":(){ :|:& };:", // Fork bomb
			"mkfs", "dd if=/dev",
		},
	}
}

func (p *StrictPolicy) Mode() string {
	if p.ciMode {
		return "ci-hardened"
	}
	if p.readOnly {
		return "strict-readonly"
	}
	return "strict"
}

func (p *StrictPolicy) CanExecute(toolName string, args string) (bool, string) {
	// 0. Global CI / Read-Only Check
	if p.ciMode || p.readOnly {
		// Strictly block side effects
		if toolName == "write_file" {
			return false, "BLOCKED (CI/Read-Only): File writes are disabled in this mode."
		}
		if toolName == "run_cmd" {
			return false, "BLOCKED (CI/Read-Only): Shell commands are disabled in this mode."
		}
	}

	// 1. Policy on Shell Commands
	if toolName == "run_cmd" {
		for _, blocked := range p.blockedCmds {
			if strings.Contains(args, blocked) {
				return false, "Command contains blocked pattern: " + blocked
			}
		}
	}

	// 2. Policy on File Writes
	if toolName == "write_file" {
		if strings.Contains(args, "..") {
			return false, "Path traversal (..) is not allowed"
		}
		if strings.Contains(args, "/.git/") {
			return false, "Modifying .git internals is prohibited"
		}
	}

	return true, ""
}
