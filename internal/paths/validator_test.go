package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSanitize(t *testing.T) {
	// Create a temporary workspace
	tmpDir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	os.Chdir(tmpDir)

	// Create a legitimate file
	os.WriteFile("safe.go", []byte("package main"), 0644)
	os.Mkdir("subdir", 0755)
	os.WriteFile("subdir/nested.go", []byte("package nested"), 0644)

	// Create a symlink that points OUTSIDE the root
	outsideDir := t.TempDir()
	secretFile := filepath.Join(outsideDir, "secret.txt")
	os.WriteFile(secretFile, []byte("secret data"), 0644)
	os.Symlink(secretFile, "bad_link.txt")

	tests := []struct {
		name      string
		input     string
		shouldErr bool
	}{
		{"Valid file", "safe.go", false},
		{"Valid nested file", "subdir/nested.go", false},
		{"Valid relative path", "./safe.go", false},
		{"Path traversal attempt", "../../../etc/passwd", true},
		{"Parent directory traversal", "../", true},
		{"Root directory traversal", "/", true},
		{"Symlink to outside", "bad_link.txt", true}, // This proves the symlink fix works
		{"Forbidden system path", "/etc/hosts", true},
		{"Forbidden dev path", "/dev/null", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Sanitize(tt.input)
			if tt.shouldErr && err == nil {
				t.Errorf("Expected error for input '%s', but got nil", tt.input)
			}
			if !tt.shouldErr && err != nil {
				t.Errorf("Unexpected error for input '%s': %v", tt.input, err)
			}
		})
	}
}

func TestValidateOutput(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		shouldErr bool
	}{
		{"Safe output", "context.md", false},
		{"Critical file: go.mod", "go.mod", true},
		{"Critical file: .git", ".git", true},
		{"Critical file: .env", ".env", true},
		{"Nested safe output", "out/context.md", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOutput(tt.input)
			if tt.shouldErr && err == nil {
				t.Errorf("Expected error for input '%s', but got nil", tt.input)
			}
		})
	}
}
