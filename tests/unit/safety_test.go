package unit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/david22573/codepicker/adapters/verifier"
	"github.com/david22573/codepicker/infra/fs"
	"github.com/david22573/codepicker/infra/pathutil"
)

// 1. Path Safety Tests
func TestPathSafety_CleanTraversals(t *testing.T) {
	cases := []struct {
		input string
		isErr bool
	}{
		{"safe.go", false},
		{"subdir/safe.go", false},
		{"../outside.txt", true},
		{"/etc/passwd", true},
		{"C:\\Windows\\System32", true},
		{"~/.ssh/id_rsa", true},
		{"", true},
	}

	for _, tc := range cases {
		_, err := pathutil.Clean(tc.input)
		if tc.isErr && err == nil {
			t.Errorf("expected error for path '%s', got nil", tc.input)
		} else if !tc.isErr && err != nil {
			t.Errorf("expected no error for path '%s', got: %v", tc.input, err)
		}
	}
}

// 2. Shadow Write & Rollback Tests
func TestShadowWrites_AndRollback(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "codepicker-shadow-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	shadow := fs.NewShadowManager(tmpDir, false)
	workspace := fs.NewWorkspaceManager(tmpDir)

	// Create a real file first
	realFile := filepath.Join(tmpDir, "test.txt")
	err = os.WriteFile(realFile, []byte("original content"), 0644)
	if err != nil {
		t.Fatalf("failed to create real file: %v", err)
	}

	// Begin transaction
	tx, err := workspace.BeginTransaction()
	if err != nil {
		t.Fatalf("failed to start transaction: %v", err)
	}
	tx.AttachShadow(shadow)

	// Backup file
	err = tx.BackupFile("test.txt")
	if err != nil {
		t.Fatalf("failed to backup file: %v", err)
	}

	// Write to shadow
	shadowPath, err := shadow.Write("test.txt", []byte("shadow content"))
	if err != nil {
		t.Fatalf("failed to write to shadow: %v", err)
	}

	// Verify shadow path exists and has shadow content
	data, err := os.ReadFile(shadowPath)
	if err != nil || string(data) != "shadow content" {
		t.Errorf("expected shadow content to be written, got: %s", string(data))
	}

	// Commit shadow file to real filesystem
	err = shadow.Commit("test.txt")
	if err != nil {
		t.Fatalf("failed to commit shadow: %v", err)
	}

	// Verify real file updated
	data, err = os.ReadFile(realFile)
	if err != nil || string(data) != "shadow content" {
		t.Errorf("expected real file to be updated, got: %s", string(data))
	}

	// Rollback the transaction to restore original content
	err = tx.Rollback()
	if err != nil {
		t.Fatalf("rollback failed: %v", err)
	}

	// Verify original content restored
	data, err = os.ReadFile(realFile)
	if err != nil || string(data) != "original content" {
		t.Errorf("expected original content to be restored after rollback, got: %s", string(data))
	}
}

// 3. Verifier Sandbox Tests
func TestVerifier_SandboxCommands(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "codepicker-verifier-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create dummy file for search replace blocks to apply safely
	err = os.WriteFile(filepath.Join(tmpDir, "dummy.txt"), []byte("hello world"), 0644)
	if err != nil {
		t.Fatalf("failed to create dummy file: %v", err)
	}

	dummyPatch := "### dummy.txt\n<<<<<<< SEARCH\nhello world\n=======\nhello verified world\n>>>>>>> REPLACE"

	pipeline := verifier.NewPipeline(tmpDir)

	// 1. Test passing command
	pipeline.Commands = []string{"echo 'hello'"}
	res, err := pipeline.Verify(context.Background(), dummyPatch)
	if err != nil {
		t.Fatalf("unexpected verification error: %v", err)
	}
	if !res.Success {
		t.Errorf("expected verification success, got failure: %s", res.Logs)
	}

	// 2. Test failing command
	pipeline.Commands = []string{"nonexistentcommand12345"}
	res, err = pipeline.Verify(context.Background(), dummyPatch)
	if err != nil {
		t.Fatalf("unexpected verification error: %v", err)
	}
	if res.Success {
		t.Error("expected verification failure for nonexistent command, got success")
	}
	if !strings.Contains(res.Logs, "nonexistentcommand") && !strings.Contains(res.Logs, "not found") && !strings.Contains(res.Logs, "exec") {
		t.Errorf("expected nonexistent command error in logs, got: %s", res.Logs)
	}
}
