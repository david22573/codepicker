package unit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/david22573/codepicker/infra/fs"
)

func setupTestEnv(t *testing.T) (string, *fs.ShadowManager) {
	dir, err := os.MkdirTemp("", "shadow-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	sm := fs.NewShadowManager(dir, false)
	return dir, sm
}

func TestShadowManager_WriteAndRead(t *testing.T) {
	dir, sm := setupTestEnv(t)
	defer os.RemoveAll(dir)

	content := []byte("package main\n\nfunc main() {}")
	relPath := "src/main.go"

	// Test Write
	shadowPath, err := sm.Write(relPath, content)
	if err != nil {
		t.Fatalf("Write() failed: %v", err)
	}

	// Verify file exists in shadow dir
	expectedPath := filepath.Join(dir, fs.ShadowDir, "src", "main.go")
	if shadowPath != expectedPath {
		t.Errorf("expected shadow path %s, got %s", expectedPath, shadowPath)
	}

	// Test Read
	readContent, err := sm.Read(relPath)
	if err != nil {
		t.Fatalf("Read() failed: %v", err)
	}

	if string(readContent) != string(content) {
		t.Errorf("Read() got %s, want %s", string(readContent), string(content))
	}
}

func TestShadowManager_Commit(t *testing.T) {
	dir, sm := setupTestEnv(t)
	defer os.RemoveAll(dir)

	content := []byte("hello world")
	relPath := "test.txt"

	_, err := sm.Write(relPath, content)
	if err != nil {
		t.Fatalf("Write() failed: %v", err)
	}

	// Test Commit
	err = sm.Commit(relPath)
	if err != nil {
		t.Fatalf("Commit() failed: %v", err)
	}

	// Verify it exists in real project root
	realPath := filepath.Join(dir, relPath)
	realContent, err := os.ReadFile(realPath)
	if err != nil {
		t.Fatalf("failed to read committed file: %v", err)
	}

	if string(realContent) != string(content) {
		t.Errorf("committed content mismatch. got %s, want %s", string(realContent), string(content))
	}

	// Verify it was removed from shadow
	_, err = sm.Read(relPath)
	if err == nil {
		t.Error("expected file to be removed from shadow after commit")
	}
}

func TestShadowManager_Clear(t *testing.T) {
	dir, sm := setupTestEnv(t)
	defer os.RemoveAll(dir)

	sm.Write("file1.go", []byte("1"))
	sm.Write("file2.go", []byte("2"))

	files, _ := sm.ListShadowFiles()
	if len(files) != 2 {
		t.Fatalf("expected 2 shadow files, got %d", len(files))
	}

	err := sm.Clear()
	if err != nil {
		t.Fatalf("Clear() failed: %v", err)
	}

	files, _ = sm.ListShadowFiles()
	if len(files) != 0 {
		t.Errorf("expected 0 shadow files after clear, got %d", len(files))
	}
}