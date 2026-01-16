package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

func GetChangedFiles(diffRef string) (map[string]bool, error) {
	var cmd *exec.Cmd

	// Logic for different git diff modes:
	// 1. "" (empty) -> unstaged changes (git diff --name-only)
	// 2. "staged"   -> staged changes (git diff --name-only --cached)
	// 3. "HEAD~1"   -> changed since ref (git diff --name-only HEAD~1)

	args := []string{"diff", "--name-only"}

	if diffRef == "staged" {
		args = append(args, "--cached")
	} else if diffRef != "" {
		args = append(args, diffRef)
	}

	cmd = exec.Command("git", args...)

	var out bytes.Buffer
	cmd.Stdout = &out
	// Capture stderr to debug git errors
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("git execution failed: %v (stderr: %s)", err, stderr.String())
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
