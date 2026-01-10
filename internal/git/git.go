package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// GetChangedFiles returns a list of files changed between the current state and the target ref.
// If diffRef is empty, it defaults to checking unstaged/staged changes vs HEAD.
func GetChangedFiles(diffRef string) (map[string]bool, error) {
	var cmd *exec.Cmd

	if diffRef == "" || diffRef == "staged" {
		// Diff staged/unstaged changes
		cmd = exec.Command("git", "diff", "--name-only")
	} else {
		// Diff against a specific branch/commit (e.g., "main" or "HEAD~1")
		cmd = exec.Command("git", "diff", "--name-only", diffRef)
	}

	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("git execution failed (is this a git repo?): %w", err)
	}

	files := make(map[string]bool)
	lines := strings.Split(out.String(), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			files[trimmed] = true
		}
	}

	return files, nil
}
