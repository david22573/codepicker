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

type Sandbox struct {
	OriginalRoot string
	SandboxRoot  string
}

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

func (s *Sandbox) Cleanup() {
	_ = os.RemoveAll(s.SandboxRoot)
}

func (s *Sandbox) ApplyPatch(patchContent []byte) error {
	return ApplySearchReplaceBlocks(s.SandboxRoot, string(patchContent))
}

func (s *Sandbox) RunGoCommand(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = s.SandboxRoot

	// Restrict to safe environment
	allowed := []string{"PATH", "HOME", "USER", "GOPATH", "GOROOT", "GOCACHE"}
	var env []string
	for _, k := range allowed {
		if v := os.Getenv(k); v != "" {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
	}
	cmd.Env = env

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
	sourceInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		destFile.Close()
		return err
	}

	if err := destFile.Sync(); err != nil {
		destFile.Close()
		return err
	}

	if err := destFile.Close(); err != nil {
		return err
	}

	return os.Chmod(dst, sourceInfo.Mode())
}

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

		tmpPath := fullPath + ".tmp"
		if err := os.WriteFile(tmpPath, []byte(newContent), 0644); err != nil {
			return err
		}
		if err := os.Rename(tmpPath, fullPath); err != nil {
			if copyErr := copyFile(tmpPath, fullPath); copyErr != nil {
				os.Remove(tmpPath)
				return fmt.Errorf("failed to atomically commit %s: rename err: %w, copy err: %v", file, err, copyErr)
			}
			os.Remove(tmpPath)
		}
	}
	return nil
}

func ApplyBlocksToString(original string, blocks string) (string, error) {
	original = strings.ReplaceAll(original, "\r\n", "\n")
	blocks = strings.ReplaceAll(blocks, "\r\n", "\n")

	lines := strings.Split(blocks, "\n")
	var state int
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
