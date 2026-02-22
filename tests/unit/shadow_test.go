package unit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/david22573/codepicker/infra/fs"
)

func TestShadowManager(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "shadow_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	mgr := fs.NewShadowManager(tempDir, false)

	// Test Write
	testPath := "src/main.go"
	content := []byte("package main\n\nfunc main() {}\n")
	
	_, err = mgr.Write(testPath, content)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Test Read
	readContent, err := mgr.Read(testPath)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if string(readContent) != string(content) {
		t.Errorf("expected %q, got %q", string(content), string(readContent))
	}

	// Test Commit
	err = mgr.Commit(testPath)
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Verify Real File Exists
	realPath := filepath.Join(tempDir, testPath)
	realContent, err := os.ReadFile(realPath)
	if err != nil {
		t.Fatalf("Failed to read committed file: %v", err)
	}
	if string(realContent) != string(content) {
		t.Errorf("committed content mismatch")
	}

	// Verify Shadow File is Removed after Commit
	_, err = mgr.Read(testPath)
	if err == nil {
		t.Errorf("expected error reading shadow file after commit, got nil")
	}
}