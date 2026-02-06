package fs

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/david22573/codepicker/domain/errors"
)

// ChangeType indicates the nature of the file operation
type ChangeType string

const (
	ChangeNew      ChangeType = "NEW"
	ChangeModified ChangeType = "MODIFIED"
	ChangeNoOp     ChangeType = "NO-OP"
)

// FileChangeSummary holds the stats for a pending operation
type FileChangeSummary struct {
	Path       string
	Type       ChangeType
	OldLines   int
	NewLines   int
	DeltaLines int
}

func (s *FileChangeSummary) String() string {
	if s.Type == ChangeNew {
		return fmt.Sprintf("[NEW]      %s (+%d lines)", s.Path, s.NewLines)
	}
	if s.Type == ChangeNoOp {
		return fmt.Sprintf("[NO-OP]    %s (content identical)", s.Path)
	}

	// Format for Modified
	sign := "+"
	if s.DeltaLines < 0 {
		sign = "" // negative number already has sign
	}
	return fmt.Sprintf("[MODIFIED] %s (Lines: %d -> %d | %s%d)", s.Path, s.OldLines, s.NewLines, sign, s.DeltaLines)
}

// Diff analyzes the differences between the shadow file and the real file
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

	if bytes.Equal(shadowContent, realContent) {
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

// Apply moves a file from shadow to real FS (The "Commit" action)
// Updated with Mutex and Sanitization for Phase 1.1
func (s *ShadowManager) Apply(relPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cleanPath, err := s.sanitizePath(relPath)
	if err != nil {
		return err
	}

	shadowPath := filepath.Join(s.ProjectRoot, ShadowDir, cleanPath)
	realPath := filepath.Join(s.ProjectRoot, cleanPath)

	content, err := os.ReadFile(shadowPath)
	if err != nil {
		return errors.NewValidation("fs.Apply", "shadow file not found: "+cleanPath)
	}

	if err := os.MkdirAll(filepath.Dir(realPath), 0755); err != nil {
		return errors.NewSystem("fs.Apply", "failed to create dirs", err)
	}

	if err := os.WriteFile(realPath, content, 0644); err != nil {
		return errors.NewSystem("fs.Apply", "failed to write real file", err)
	}

	_ = os.Remove(shadowPath)
	return nil
}

// Helper for Diff
func countLines(data []byte) int {
	sc := bufio.NewScanner(bytes.NewReader(data))
	count := 0
	for sc.Scan() {
		count++
	}
	return count
}
