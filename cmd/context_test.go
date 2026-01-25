package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestContextGen verifies that the 'ctx gen' command correctly concatenates files
func TestContextGen(t *testing.T) {
	// 1. Setup temporary source directory
	tmpDir := t.TempDir()
	file1 := filepath.Join(tmpDir, "main.go")
	file2 := filepath.Join(tmpDir, "utils.go")

	if err := os.WriteFile(file1, []byte("package main\nfunc main() {}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file2, []byte("package main\nfunc util() {}"), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Setup output file
	outFile := filepath.Join(tmpDir, "context.md")

	// 3. Configure the command flags manually (simulating CLI input)
	// We need to set the package-level variables that the command uses
	srcDir = tmpDir
	ctxOut = outFile
	ctxDryRun = false
	ctxTokens = false
	minify = false

	// 4. Run the command logic
	// We call runContextScan directly or via the command RunE
	cmd := &cobra.Command{}
	err := runContextScan(cmd, "Concat")
	if err != nil {
		t.Fatalf("runContextScan failed: %v", err)
	}

	// 5. Verify Output
	content, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	strContent := string(content)
	if !strings.Contains(strContent, "## File: main.go") {
		t.Error("Output missing main.go header")
	}
	if !strings.Contains(strContent, "func main() {}") {
		t.Error("Output missing main.go content")
	}
	if !strings.Contains(strContent, "## File: utils.go") {
		t.Error("Output missing utils.go header")
	}
}

// TestContextTree verifies the tree generation logic
func TestContextTree(t *testing.T) {
	tmpDir := t.TempDir()
	os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "subdir", "test.txt"), []byte("data"), 0644)

	srcDir = tmpDir
	ctxOut = "" // No file output, just stdout
	ctxCopy = false

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := &cobra.Command{}
	err := runContextScan(cmd, "Tree")

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("Tree generation failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "subdir") {
		t.Error("Tree output missing subdirectory")
	}
	if !strings.Contains(output, "test.txt") {
		t.Error("Tree output missing file")
	}
}
