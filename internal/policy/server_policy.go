package policy

import "time"

// Server defines the strict policy for the HTTP Daemon.
// By default, it blocks shell access to prevent remote code execution vulnerabilities
// via the API, unless specifically enabled by the user.
var Server = ExecutionPolicy{
	Name:           "Server",
	Mode:           LevelStrict,
	AllowShell:     false, // HARD DENY by default for web agents
	AllowFileWrite: true,  // Agents need to write code
	MaxRuntime:     5 * time.Minute,
	RequireReason:  false, // Reasoning is logged but not interactively prompted
	AllowedBinaries: []string{
		"ls", "grep", "cat", // minimal read-only ops
	},
}
