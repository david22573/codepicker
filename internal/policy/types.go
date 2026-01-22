package policy

import (
	"fmt"
	"strings"
	"time"
)

type Level string

const (
	LevelStrict      Level = "strict"      // Deny unless explicitly allowed
	LevelInteractive Level = "interactive" // Ask user for confirmation
	LevelAuto        Level = "auto"        // Allow specific safe actions, block others
)

type ExecutionPolicy struct {
	Name            string
	Mode            Level
	AllowShell      bool
	AllowFileWrite  bool
	AllowedBinaries []string
	MaxRuntime      time.Duration
	RequireReason   bool // If true, agent must explain why before tool use
}

var (
	// Interactive: The human is watching. Allow most things, but prompt for shell/writes.
	Interactive = ExecutionPolicy{
		Name:           "Interactive",
		Mode:           LevelInteractive,
		AllowShell:     true,
		AllowFileWrite: true,
		MaxRuntime:     1 * time.Hour,
		RequireReason:  true,
	}

	// Batch: Headless. Safer defaults. No sudo, no weird binaries.
	// Phase 2: Align Batch policy with execution reality (added gofmt)
	Batch = ExecutionPolicy{
		Name:            "Batch",
		Mode:            LevelAuto,
		AllowShell:      false,
		AllowFileWrite:  true,
		AllowedBinaries: []string{"ls", "grep", "find", "go", "npm", "gofmt"},
		MaxRuntime:      15 * time.Minute,
	}

	// Architect: Read-only mode for auditing.
	Architect = ExecutionPolicy{
		Name:           "Architect",
		Mode:           LevelStrict,
		AllowShell:     false,
		AllowFileWrite: false,
		MaxRuntime:     10 * time.Minute,
	}
)

func (p ExecutionPolicy) ValidateCommand(command string) error {
	if !p.AllowShell {
		return fmt.Errorf("shell execution is disabled by policy '%s'", p.Name)
	}

	parts := strings.Fields(command)
	if len(parts) == 0 {
		return fmt.Errorf("empty command")
	}
	binary := parts[0]

	if p.Mode == LevelStrict || p.Mode == LevelAuto {
		allowed := false
		for _, b := range p.AllowedBinaries {
			if b == binary {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("binary '%s' is not in the allowlist for policy '%s'", binary, p.Name)
		}
	}

	return nil
}
