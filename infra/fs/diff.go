package fs

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
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
	shadowPath := filepath.Join(s.ProjectRoot, ShadowDir, relPath)
	realPath := filepath.Join(s.ProjectRoot, relPath)

	// 1. Read Shadow (Must exist)
	shadowContent, err := os.ReadFile(shadowPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("shadow file does not exist: %s", relPath)
		}
		return nil, err
	}

	// 2. Read Real (Might not exist)
	realContent, err := os.ReadFile(realPath)
	isNew := os.IsNotExist(err)

	shadowLines := countLines(shadowContent)

	if isNew {
		return &FileChangeSummary{
			Path:       relPath,
			Type:       ChangeNew,
			OldLines:   0,
			NewLines:   shadowLines,
			DeltaLines: shadowLines,
		}, nil
	}

	// 3. Compare for Modification
	if bytes.Equal(shadowContent, realContent) {
		return &FileChangeSummary{
			Path:     relPath,
			Type:     ChangeNoOp,
			OldLines: shadowLines,
			NewLines: shadowLines,
		}, nil
	}

	realLines := countLines(realContent)
	return &FileChangeSummary{
		Path:       relPath,
		Type:       ChangeModified,
		OldLines:   realLines,
		NewLines:   shadowLines,
		DeltaLines: shadowLines - realLines,
	}, nil
}

func countLines(data []byte) int {
	sc := bufio.NewScanner(bytes.NewReader(data))
	count := 0
	for sc.Scan() {
		count++
	}
	return count
}
