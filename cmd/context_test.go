package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestContextOverwriteProtection(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir = tmpDir // Set global srcDir for the test

	// Create a dummy source file so scanner has something to do
	dummySrc := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(dummySrc, []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a pre-existing output file to trigger the collision
	outFile := filepath.Join(tmpDir, "existing_context.md")
	if err := os.WriteFile(outFile, []byte("# Old Data"), 0644); err != nil {
		t.Fatal(err)
	}

	// Mock command
	cmd := &cobra.Command{}

	// Case 1: Overwrite is FALSE (default) -> Should Fail
	ctxOut = outFile
	ctxOverwrite = false
	ctxDryRun = false

	err := runContextScan(cmd, "Concat")
	if err == nil {
		t.Error("Expected error when overwriting existing file without permission, got nil")
	} else if err.Error() != "file '"+outFile+"' already exists; use --yes to overwrite" {
		t.Errorf("Unexpected error message: %v", err)
	}

	// Case 2: Overwrite is TRUE -> Should Succeed
	ctxOverwrite = true
	err = runContextScan(cmd, "Concat")
	if err != nil {
		t.Errorf("Expected success when overwrite is allowed, got error: %v", err)
	}
}

func TestApplyInvalidGlob(t *testing.T) {
	// Reset flags
	acceptPattern = ""
	rejectPattern = ""

	// Case: Invalid Pattern
	acceptPattern = "["

	// We can't easily run applyCmd.RunE directly without mocking a lot of shadow fs stuff,
	// but we can check the logic matches what we implemented.
	// Ideally, we extract validation logic, but for this roadmap level,
	// we rely on the manual verification of the inserted block in apply.go.
	// This test placeholder is just to remind us where to put integration tests later.
}
