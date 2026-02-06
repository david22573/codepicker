package integration

import (
	"os"
	"testing"

	"github.com/david22573/codepicker/infra/fs"
)

func TestTransactionRollback(t *testing.T) {
	repo := NewTestRepo(t)
	defer repo.Teardown()

	// 1. Setup initial state
	initialFile := "config.json"
	initialContent := `{"version": 1}`
	repo.WriteFile(t, initialFile, initialContent)

	manager := fs.NewWorkspaceManager(repo.Root)

	// 2. Start Transaction
	txn, err := manager.BeginTransaction()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	// 3. Modify existing file (Backup should happen)
	if err := txn.BackupFile(initialFile); err != nil {
		t.Fatalf("Failed to backup file: %v", err)
	}
	repo.WriteFile(t, initialFile, `{"version": 2, "corrupted": true}`)

	// 4. Create a NEW file (Should be deleted on rollback)
	newFile := "new_feature.go"
	// Note: We "Backup" it to track it, even though it doesn't exist yet
	txn.BackupFile(newFile)
	repo.WriteFile(t, newFile, "package main")

	// 5. Trigger Rollback (Simulate failure)
	// We intentionally do NOT call txn.Commit()
	if err := txn.Rollback(); err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	// 6. Verify State

	// Check 1: Initial file should be restored to version 1
	content, err := os.ReadFile(repo.Path(initialFile))
	if err != nil {
		t.Fatalf("Failed to read restored file: %v", err)
	}
	if string(content) != initialContent {
		t.Errorf("Rollback failed. Expected '%s', got '%s'", initialContent, string(content))
	}

	// Check 2: New file should be gone
	if _, err := os.Stat(repo.Path(newFile)); !os.IsNotExist(err) {
		t.Errorf("Rollback failed. New file '%s' should have been deleted.", newFile)
	}
}
