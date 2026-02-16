package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/david22573/codepicker/domain/errors"
)

const ShadowDir = ".codepicker/shadow"

type ShadowManager struct {
	ProjectRoot string
	DryRun      bool
	mu          sync.RWMutex
}

func NewShadowManager(root string, dryRun bool) *ShadowManager {
	absRoot, _ := filepath.Abs(root)
	return &ShadowManager{
		ProjectRoot: absRoot,
		DryRun:      dryRun,
	}
}

// Write saves content to the shadow directory.
func (s *ShadowManager) Write(relPath string, content []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cleanPath, err := s.sanitizePath(relPath)
	if err != nil {
		return "", err
	}

	shadowPath := filepath.Join(s.ProjectRoot, ShadowDir, cleanPath)

	if err := os.MkdirAll(filepath.Dir(shadowPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create shadow dirs: %w", err)
	}

	if err := os.WriteFile(shadowPath, content, 0644); err != nil {
		return "", fmt.Errorf("failed to write shadow file: %w", err)
	}

	return shadowPath, nil
}

// Read retrieves content from the shadow directory if it exists.
func (s *ShadowManager) Read(relPath string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cleanPath, err := s.sanitizePath(relPath)
	if err != nil {
		return nil, err
	}

	shadowPath := filepath.Join(s.ProjectRoot, ShadowDir, cleanPath)
	return os.ReadFile(shadowPath)
}

// Commit moves a single file from shadow to the real filesystem.
func (s *ShadowManager) Commit(relPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.DryRun {
		return nil
	}

	cleanPath, err := s.sanitizePath(relPath)
	if err != nil {
		return err
	}

	shadowPath := filepath.Join(s.ProjectRoot, ShadowDir, cleanPath)
	realPath := filepath.Join(s.ProjectRoot, cleanPath)

	content, err := os.ReadFile(shadowPath)
	if err != nil {
		return errors.NewValidation("fs.Commit", "shadow file not found: "+cleanPath)
	}

	if err := os.MkdirAll(filepath.Dir(realPath), 0755); err != nil {
		return errors.NewSystem("fs.Commit", "failed to create dirs", err)
	}

	if err := os.WriteFile(realPath, content, 0644); err != nil {
		return errors.NewSystem("fs.Commit", "failed to write real file", err)
	}

	// Remove from shadow after successful commit to keep state clean
	_ = os.Remove(shadowPath)
	return nil
}

// ExportTo copies all shadow files to a specific target directory.
// This is used to hydrate a temporary sandbox with the "pending" state.
func (s *ShadowManager) ExportTo(targetRoot string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	shadowRoot := filepath.Join(s.ProjectRoot, ShadowDir)

	// If no shadow dir exists, nothing to export
	if _, err := os.Stat(shadowRoot); os.IsNotExist(err) {
		return nil
	}

	return filepath.Walk(shadowRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		// Get path relative to .codepicker/shadow
		rel, err := filepath.Rel(shadowRoot, path)
		if err != nil {
			return err
		}

		// Determine destination in the target root
		destPath := filepath.Join(targetRoot, rel)

		// Create dest dir
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}

		// Copy content
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(destPath, content, 0644)
	})
}

// Clear wipes the entire shadow directory.
func (s *ShadowManager) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	shadowRoot := filepath.Join(s.ProjectRoot, ShadowDir)
	// Safety check: ensure we are actually deleting the shadow dir
	if filepath.Base(shadowRoot) != "shadow" {
		return fmt.Errorf("safety check failed: refusing to clear %s", shadowRoot)
	}
	return os.RemoveAll(shadowRoot)
}

func (s *ShadowManager) ListShadowFiles() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	shadowRoot := filepath.Join(s.ProjectRoot, ShadowDir)
	var files []string

	if _, err := os.Stat(shadowRoot); os.IsNotExist(err) {
		return files, nil
	}

	err := filepath.Walk(shadowRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			rel, _ := filepath.Rel(shadowRoot, path)
			files = append(files, rel)
		}
		return nil
	})

	return files, err
}

// Diff compares shadow file vs real file.
func (s *ShadowManager) Diff(relPath string) (*FileChangeSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cleanPath, err := s.sanitizePath(relPath)
	if err != nil {
		return nil, err
	}

	shadowPath := filepath.Join(s.ProjectRoot, ShadowDir, cleanPath)
	realPath := filepath.Join(s.ProjectRoot, cleanPath)

	shadowContent, err := os.ReadFile(shadowPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("shadow file does not exist: %s", cleanPath)
		}
		return nil, err
	}

	realContent, err := os.ReadFile(realPath)
	isNew := os.IsNotExist(err)

	shadowLines := countLines(shadowContent)

	if isNew {
		return &FileChangeSummary{
			Path:       cleanPath,
			Type:       ChangeNew,
			OldLines:   0,
			NewLines:   shadowLines,
			DeltaLines: shadowLines,
		}, nil
	}

	// Simple byte comparison
	if string(shadowContent) == string(realContent) {
		return &FileChangeSummary{
			Path:     cleanPath,
			Type:     ChangeNoOp,
			OldLines: shadowLines,
			NewLines: shadowLines,
		}, nil
	}

	realLines := countLines(realContent)
	return &FileChangeSummary{
		Path:       cleanPath,
		Type:       ChangeModified,
		OldLines:   realLines,
		NewLines:   shadowLines,
		DeltaLines: shadowLines - realLines,
	}, nil
}

func (s *ShadowManager) sanitizePath(relPath string) (string, error) {
	clean := filepath.Clean(relPath)
	if filepath.IsAbs(clean) {
		return "", fmt.Errorf("absolute paths not allowed")
	}
	return clean, nil
}

func countLines(data []byte) int {
	count := 0
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			count++
		}
	}
	return count
}

type ChangeType string

const (
	ChangeNew      ChangeType = "NEW"
	ChangeModified ChangeType = "MODIFIED"
	ChangeNoOp     ChangeType = "NO-OP"
)

type FileChangeSummary struct {
	Path       string
	Type       ChangeType
	OldLines   int
	NewLines   int
	DeltaLines int
}
