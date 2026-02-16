package fs

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Sandbox represents an isolated copy of the project for verification.
type Sandbox struct {
	OriginalRoot string
	SandboxRoot  string
}

// NewSandbox creates a temporary directory and syncs the project files to it.
// It explicitly excludes .git, .codepicker, and other heavy artifacts.
func NewSandbox(projectRoot string) (*Sandbox, error) {
	tmpDir, err := os.MkdirTemp("", "codepicker-sandbox-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	s := &Sandbox{
		OriginalRoot: projectRoot,
		SandboxRoot:  tmpDir,
	}

	// Copy files from original project to sandbox
	if err := s.syncFiles(); err != nil {
		_ = os.RemoveAll(tmpDir) // Clean up on failure
		return nil, fmt.Errorf("failed to sync files to sandbox: %w", err)
	}

	return s, nil
}

// Cleanup removes the temporary sandbox directory.
func (s *Sandbox) Cleanup() {
	_ = os.RemoveAll(s.SandboxRoot)
}

// ApplyPatch runs 'git apply' inside the sandbox.
func (s *Sandbox) ApplyPatch(patchContent []byte) error {
	patchPath := filepath.Join(s.SandboxRoot, "temp.diff")
	if err := os.WriteFile(patchPath, patchContent, 0644); err != nil {
		return fmt.Errorf("failed to write patch file: %w", err)
	}

	cmd := exec.Command("git", "apply", "temp.diff")
	cmd.Dir = s.SandboxRoot

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("patch failed: %v\nOutput: %s", err, string(out))
	}

	return nil
}

// RunGoCommand executes a go command (test, build, vet) inside the sandbox.
func (s *Sandbox) RunGoCommand(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = s.SandboxRoot
	cmd.Env = os.Environ() // Inherit environment (PATH, GOPATH, etc.)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), err
	}

	return string(out), nil
}

// syncFiles copies the source code to the sandbox.
// OPTIMIZATION: Uses hard links (os.Link) where possible to avoid I/O overhead.
func (s *Sandbox) syncFiles() error {
	return filepath.Walk(s.OriginalRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("access error at %s: %w", path, err)
		}

		relPath, _ := filepath.Rel(s.OriginalRoot, path)
		if relPath == "." {
			return nil
		}

		if info.IsDir() {
			name := info.Name()
			if (strings.HasPrefix(name, ".") && name != ".") || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			// Create directory in sandbox
			return os.MkdirAll(filepath.Join(s.SandboxRoot, relPath), info.Mode())
		}

		// Skip hidden files (like .env or .DS_Store)
		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		// Define destination path
		destPath := filepath.Join(s.SandboxRoot, relPath)

		// 1. Try Hard Link (Fastest)
		// This creates a new directory entry pointing to the same data on disk.
		// It avoids reading/writing file content.
		err = os.Link(path, destPath)
		if err == nil {
			return nil // Success
		}

		// 2. Fallback to Copy (Slower)
		// Necessary if cross-device link or filesystem doesn't support links.
		return copyFile(path, destPath)
	})
}

// copyFile is a helper to copy file content when hard linking fails.
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}
