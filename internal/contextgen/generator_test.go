package contextgen

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/david22573/codepicker/internal/logger"
)

func TestGenerate(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "utils.go"), []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}

	opts := Options{
		SrcDir: tmpDir,
		Minify: false,
	}

	result, err := Generate(context.Background(), opts, &logger.NoOpLogger{})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if !strings.Contains(result, "main.go") {
		t.Error("Expected result to contain main.go")
	}

	if !strings.Contains(result, "utils.go") {
		t.Error("Expected result to contain utils.go")
	}
}

func TestGenerateFocusMode(t *testing.T) {
	tmpDir := t.TempDir()

	mainFile := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(mainFile, []byte("package main\nfunc main() {}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "other.go"), []byte("package other"), 0644); err != nil {
		t.Fatal(err)
	}

	opts := Options{
		SrcDir:     tmpDir,
		FocusFiles: []string{mainFile},
		Minify:     false,
	}

	result, err := Generate(context.Background(), opts, &logger.NoOpLogger{})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if !strings.Contains(result, "main.go") {
		t.Error("Expected result to contain main.go")
	}

	if strings.Contains(result, "other.go") {
		t.Error("Expected result to NOT contain other.go in focus mode")
	}
}
