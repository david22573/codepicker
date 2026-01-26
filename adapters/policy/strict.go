package policy

import (
	"strings"
)

type StrictPolicy struct {
	blockedCmds []string
}

func NewStrictPolicy() *StrictPolicy {
	return &StrictPolicy{
		blockedCmds: []string{
			"rm -rf /", "rm -rf ~", "sudo", "su ", ":(){ :|:& };:", // Fork bomb
			"mkfs", "dd if=/dev",
		},
	}
}

func (p *StrictPolicy) Mode() string {
	return "strict"
}

func (p *StrictPolicy) CanExecute(toolName string, args string) (bool, string) {
	// 1. Policy on Shell Commands
	if toolName == "run_cmd" {
		// Simple string check; a production version might parse the AST of the shell command
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

	// Default allow
	return true, ""
}
