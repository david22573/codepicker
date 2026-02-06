package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestRepo represents a temporary git repository for integration testing.
type TestRepo struct {
	Root string
}

// NewTestRepo creates a fresh temporary directory with an initialized git repo.
func NewTestRepo(t *testing.T) *TestRepo {
	t.Helper()

	// Create temp dir
	dir, err := os.MkdirTemp("", "codepicker-integration-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Initialize Git
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		os.RemoveAll(dir)
		t.Fatalf("Failed to git init: %v", err)
	}

	// Configure dummy user for git commits to work
	configEmail := exec.Command("git", "config", "user.email", "test@codepicker.local")
	configEmail.Dir = dir
	configEmail.Run()

	configName := exec.Command("git", "config", "user.name", "Test Bot")
	configName.Dir = dir
	configName.Run()

	return &TestRepo{Root: dir}
}

// Teardown cleans up the temporary directory.
func (r *TestRepo) Teardown() {
	os.RemoveAll(r.Root)
}

// Path returns the absolute path to a file inside the test repo.
func (r *TestRepo) Path(relPath string) string {
	return filepath.Join(r.Root, relPath)
}

// WriteFile creates a file in the test repo with given content.
func (r *TestRepo) WriteFile(t *testing.T, relPath, content string) {
	t.Helper()
	fullPath := r.Path(relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatalf("Failed to create dirs: %v", err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
}
