package policy

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/infra/validation"
)

// CIPolicy enforces strict enterprise boundaries for CI/CD environments.
// It prohibits arbitrary code execution and restricts filesystem writes to specific targets.
type CIPolicy struct {
	AllowedCommands []string
	AllowedPaths    []string // Specific files or directories the agent is permitted to edit
}

// NewCIPolicy initializes a strict execution boundary.
func NewCIPolicy(allowedCmds, allowedPaths []string) *CIPolicy {
	var cleanPaths []string
	for _, p := range allowedPaths {
		cleanPaths = append(cleanPaths, filepath.Clean(p))
	}

	return &CIPolicy{
		AllowedCommands: allowedCmds,
		AllowedPaths:    cleanPaths,
	}
}

// CanExecute intercepts tool dispatch and asserts authorization.
func (p *CIPolicy) CanExecute(toolName, args string) (bool, string) {
	switch toolName {
	case "run_cmd":
		var input struct {
			Command string `json:"command"`
		}
		if err := validation.DecodeStrict(args, &input); err != nil {
			return false, "malformed command payload"
		}
		
		cmdBase := strings.Split(strings.TrimSpace(input.Command), " ")[0]
		for _, allowed := range p.AllowedCommands {
			if cmdBase == allowed {
				return true, ""
			}
		}
		return false, fmt.Sprintf("CI Hardened Mode: command '%s' is not in the whitelist", cmdBase)

	case "write_file", "edit_file":
		var input struct {
			Path string `json:"path"`
		}
		if err := validation.DecodeStrict(args, &input); err != nil {
			return false, "malformed file payload"
		}
		
		target := filepath.Clean(input.Path)
		for _, allowed := range p.AllowedPaths {
			// Exact match or prefix match (if allowed is a directory)
			if target == allowed || strings.HasPrefix(target, allowed+string(filepath.Separator)) {
				return true, ""
			}
		}
		return false, fmt.Sprintf("CI Hardened Mode: write access to '%s' is prohibited. You may only edit targeted files.", target)

	case "read_file", "list_dir", "search_code", "search_definition", "read_skeleton":
		// Read-only operations are generally safe, but we prevent directory traversal
		if strings.Contains(args, "..") {
			return false, "directory traversal attempts are blocked"
		}
		return true, ""
	}

	return false, fmt.Sprintf("Unknown tool '%s' blocked by CI Policy", toolName)
}