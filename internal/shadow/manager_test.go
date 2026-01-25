package shadow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/david22573/codepicker/internal/shadow"
)

func TestShadowManager(t *testing.T) {
	// Create a fake project root
	srcRoot := t.TempDir()

	// Create a real file in src
	realFile := "main.go"
	originalContent := "package main\n\nfunc main() {}"
	if err := os.WriteFile(filepath.Join(srcRoot, realFile), []byte(originalContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Init Manager
	mgr, err := shadow.NewManager(srcRoot)
	if err != nil {
		t.Fatalf("Failed to init manager: %v", err)
	}
	defer mgr.Cleanup()

	t.Run("Write and Verify Shadow File", func(t *testing.T) {
		newContent := "package main\n\nfunc main() { println(\"Shadow\") }"
		shadowPath, err := mgr.WriteFile(realFile, []byte(newContent))
		if err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}

		// Check file exists in shadow dir
		if _, err := os.Stat(shadowPath); os.IsNotExist(err) {
			t.Errorf("Shadow file not created at %s", shadowPath)
		}

		// Check manifest
		changes := mgr.GetManifestChanges()
		if _, ok := changes[realFile]; !ok {
			t.Error("Manifest does not contain entry for main.go")
		}
	})

	t.Run("Diff Generation", func(t *testing.T) {
		// Since we wrote a shadow file above, Diff should show changes
		diff, err := mgr.PreviewDiff(realFile)
		if err != nil {
			t.Fatalf("PreviewDiff failed: %v", err)
		}

		// Note: This relies on the system 'diff' utility.
		// If 'diff' is missing, the command fails. We check if diff output contains expected changes.
		if _, err := os.Stat("/usr/bin/diff"); os.IsNotExist(err) && os.Getenv("CI") == "" {
			t.Log("Skipping exact diff output check (diff utility might be missing)")
		} else {
			if !strings.Contains(diff, "println") {
				t.Errorf("Diff expected to contain 'println', got:\n%s", diff)
			}
		}
	})

	t.Run("Apply Atomic", func(t *testing.T) {
		backupPath, err := mgr.ApplyAtomic(realFile)
		if err != nil {
			t.Fatalf("ApplyAtomic failed: %v", err)
		}

		// Check Source File Updated
		content, _ := os.ReadFile(filepath.Join(srcRoot, realFile))
		if !strings.Contains(string(content), "Shadow") {
			t.Error("Source file was not updated with shadow content")
		}

		// Check Backup Exists
		if _, err := os.Stat(backupPath); os.IsNotExist(err) {
			t.Error("Backup file was not created")
		}

		// Check Manifest Cleared
		changes := mgr.GetManifestChanges()
		if _, ok := changes[realFile]; ok {
			t.Error("Manifest entry should have been removed after apply")
		}
	})

	t.Run("Restore Backup", func(t *testing.T) {
		// We have a backup from the previous step (main.go.bak)
		backupPath := filepath.Join(srcRoot, realFile+".bak")

		err := mgr.Restore(realFile, backupPath)
		if err != nil {
			t.Fatalf("Restore failed: %v", err)
		}

		// Verify content reverted
		content, _ := os.ReadFile(filepath.Join(srcRoot, realFile))
		if strings.Contains(string(content), "Shadow") {
			t.Error("Restore failed to revert content")
		}
		if !strings.Contains(string(content), "func main() {}") {
			t.Error("Restore failed to match original content")
		}

		// Verify backup deleted
		if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
			t.Error("Backup file should be deleted after restore")
		}
	})
}
