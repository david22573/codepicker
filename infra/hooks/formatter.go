package hooks

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"
)

// Formatter defines a supported code formatter configuration
type Formatter struct {
	Name    string
	Command string
	Args    []string
}

// SupportedFormatters maps extensions to their respective tools
var SupportedFormatters = map[string]Formatter{
	".go":   {Name: "gofmt", Command: "gofmt", Args: []string{"-w"}},
	".js":   {Name: "prettier", Command: "npx", Args: []string{"prettier", "--write"}},
	".ts":   {Name: "prettier", Command: "npx", Args: []string{"prettier", "--write"}},
	".json": {Name: "prettier", Command: "npx", Args: []string{"prettier", "--write"}},
	".py":   {Name: "black", Command: "black", Args: []string{}},
}

// RunFormatter attempts to format a specific file based on its extension
func RunFormatter(ctx context.Context, path string) error {
	ext := filepath.Ext(path)
	formatter, exists := SupportedFormatters[ext]
	if !exists {
		return nil // No formatter defined for this type
	}

	// Verify tool exists in PATH
	if _, err := exec.LookPath(formatter.Command); err != nil {
		fmt.Printf("⚠️  Skipping format for %s: '%s' not found in PATH.\n", filepath.Base(path), formatter.Command)
		return nil
	}

	// Execute with timeout to prevent hangs
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	fullArgs := append(formatter.Args, path)
	cmd := exec.CommandContext(ctx, formatter.Command, fullArgs...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Return error but include output for debugging
		return fmt.Errorf("formatter %s failed: %v\nOutput: %s", formatter.Name, err, string(output))
	}

	fmt.Printf("✨ Formatted %s using %s\n", filepath.Base(path), formatter.Name)
	return nil
}
