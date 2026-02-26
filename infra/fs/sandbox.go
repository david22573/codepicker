package fs

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Sandbox represents an isolated copy of the project for verification.
type Sandbox struct {
	OriginalRoot string
	SandboxRoot  string
}

// NewSandbox creates a temporary directory and syncs the project files to it.
func NewSandbox(projectRoot string) (*Sandbox, error) {
	tmpDir, err := os.MkdirTemp("", "codepicker-sandbox-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	s := &Sandbox{
		OriginalRoot: projectRoot,
		SandboxRoot:  tmpDir,
	}

	if err := s.syncFiles(); err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("failed to sync files to sandbox: %w", err)
	}

	return s, nil
}

// Cleanup removes the temporary sandbox directory.
func (s *Sandbox) Cleanup() {
	_ = os.RemoveAll(s.SandboxRoot)
}

// ApplyPatch runs native block patching inside the sandbox.
func (s *Sandbox) ApplyPatch(patchContent []byte) error {
	return ApplySearchReplaceBlocks(s.SandboxRoot, string(patchContent))
}

// RunGoCommand executes a go command (test, build, vet) inside the sandbox.
func (s *Sandbox) RunGoCommand(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = s.SandboxRoot
	cmd.Env = os.Environ()

	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), err
	}

	return string(out), nil
}

func (s *Sandbox) syncFiles() error {
	return filepath.Walk(s.OriginalRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("access error at %s: %w", path, err)
		}

		relPath, _ := filepath.Rel(s.OriginalRoot, path)
		if relPath == "." {
			return nil
		}

		if info.IsDir() {
			name := info.Name()
			if (strings.HasPrefix(name, ".") && name != ".") || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(s.SandboxRoot, relPath), info.Mode())
		}

		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		destPath := filepath.Join(s.SandboxRoot, relPath)

		err = os.Link(path, destPath)
		if err == nil {
			return nil
		}

		return copyFile(path, destPath)
	})
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// ApplySearchReplaceBlocks parses a multi-file patch string and applies it to the filesystem atomically.
func ApplySearchReplaceBlocks(rootDir string, text string) error {
	lines := strings.Split(text, "\n")
	var currentFile string
	fileBlocks := make(map[string][]string)

	for _, line := range lines {
		if strings.HasPrefix(line, "### ") {
			currentFile = strings.TrimSpace(strings.TrimPrefix(line, "### "))
			continue
		}
		if currentFile != "" {
			fileBlocks[currentFile] = append(fileBlocks[currentFile], line)
		}
	}

	if len(fileBlocks) == 0 {
		return fmt.Errorf("malformed patch: no '### filepath' markers found")
	}

	for file, blockLines := range fileBlocks {
		fullPath := filepath.Join(rootDir, file)
		contentBytes, err := os.ReadFile(fullPath)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", file, err)
		}

		blocksStr := strings.Join(blockLines, "\n")
		newContent, err := ApplyBlocksToString(string(contentBytes), blocksStr)
		if err != nil {
			return fmt.Errorf("failed to apply blocks to %s: %w", file, err)
		}

		// Write atomically to avoid corrupted files on crash
		tmpPath := fullPath + ".tmp"
		if err := os.WriteFile(tmpPath, []byte(newContent), 0644); err != nil {
			return err
		}
		if err := os.Rename(tmpPath, fullPath); err != nil {
			os.Remove(tmpPath) // Cleanup tmp if rename fails
			return fmt.Errorf("failed to atomically commit %s: %w", file, err)
		}
	}
	return nil
}

// ApplyBlocksToString takes the original file content and the LLM's SEARCH/REPLACE blocks
// and returns the modified content enforcing strict exact matching.
func ApplyBlocksToString(original string, blocks string) (string, error) {
	original = strings.ReplaceAll(original, "\r\n", "\n")
	blocks = strings.ReplaceAll(blocks, "\r\n", "\n")

	lines := strings.Split(blocks, "\n")
	var state int // 0: looking for <<<<, 1: search, 2: replace
	var search, replace []string
	result := original

	for _, line := range lines {
		if line == "<<<<" {
			state = 1
			search = nil
			replace = nil
			continue
		}
		if line == "====" && state == 1 {
			state = 2
			continue
		}
		if line == ">>>>" && state == 2 {
			newResult, err := exactReplace(result, search, replace)
			if err != nil {
				return "", err
			}
			result = newResult
			state = 0
			continue
		}

		if state == 1 {
			search = append(search, line)
		} else if state == 2 {
			replace = append(replace, line)
		}
	}

	if state != 0 {
		return "", fmt.Errorf("malformed SEARCH/REPLACE blocks: missing terminating >>>>")
	}

	return result, nil
}

// exactReplace mandates an exact character-for-character match of the search block
// to prevent logic corruption via fuzzy matching.
func exactReplace(content string, search, replace []string) (string, error) {
	searchStr := strings.Join(search, "\n")
	replaceStr := strings.Join(replace, "\n")

	if searchStr == "" || strings.TrimSpace(searchStr) == "" {
		return "", fmt.Errorf("SEARCH block contains only whitespace or is empty")
	}

	count := strings.Count(content, searchStr)
	if count == 0 {
		return "", fmt.Errorf("SEARCH block not found exactly as written. Ensure whitespace and indentation match the file perfectly")
	}
	if count > 1 {
		return "", fmt.Errorf("SEARCH block matches multiple locations. Add more context lines to make it unique")
	}

	return strings.Replace(content, searchStr, replaceStr, 1), nil
}