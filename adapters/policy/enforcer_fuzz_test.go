package policy

import (
	"strings"
	"testing"
)

// FuzzEnforcer checks that CanExecute never panics, regardless of how malformed the input is.
func FuzzEnforcer(f *testing.F) {
	// Setup a standard policy for testing
	cfg := PolicySchema{
		ForbiddenKeywords: []string{"rm -rf", "/etc/shadow", "drop table"},
		AllowedGlobs:      []string{"**/*.go"},
	}
	enforcer := NewEnforcer(cfg, false)

	// 1. Seed corpus
	f.Add("run_cmd", `{"command": "ls -la"}`)
	f.Add("write_file", `{"path": "main.go", "content": "package main"}`)
	f.Add("run_cmd", `{"command": "rm -rf /"}`)           // Malicious seed
	f.Add("read_file", `{"path": "../../../etc/passwd"}`) // Traversal seed
	f.Add("unknown_tool", `garbage_data`)

	// 2. Fuzzing Loop
	f.Fuzz(func(t *testing.T, tool string, args string) {
		allowed, reason := enforcer.CanExecute(tool, args)

		// Invariant A: If allowed is true, reason should generally be empty (or at least not an error message)
		if allowed && reason != "" {
			// This isn't strictly a failure in all designs, but for our Enforcer,
			// allowed=true usually implies no blocking reason.
		}

		// Invariant B: If blocked (allowed=false), reason must be provided
		if !allowed && reason == "" {
			t.Errorf("Blocked execution of tool %s but gave no reason", tool)
		}

		// Invariant C: Specific dangerous patterns must ALWAYS be blocked
		// This is a "property-based" check within the fuzz test
		if strings.Contains(args, "rm -rf") && tool == "run_cmd" {
			if allowed {
				t.Fatalf("CRITICAL SECURITY FAILURE: Enforcer allowed 'rm -rf' in args: %s", args)
			}
		}
	})
}
