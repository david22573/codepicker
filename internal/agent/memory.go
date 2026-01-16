package agent

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/david22573/codepicker/internal/database"
)

// WorkingMemory now delegates to SQLite for persistence
type WorkingMemory struct {
	SrcRoot string
	Store   *database.Store
}

// NewMemory now requires the database store
func NewMemory(srcRoot string, store *database.Store) *WorkingMemory {
	return &WorkingMemory{
		SrcRoot: srcRoot,
		Store:   store,
	}
}

// Add reads a file from disk and saves it to the SQLite working memory
func (m *WorkingMemory) Add(relPath string) error {
	fullPath := filepath.Join(m.SrcRoot, relPath)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	return m.Store.UpdateWorkingMemory(relPath, string(content))
}

func (m *WorkingMemory) Remove(relPath string) {
	m.Store.RemoveFromMemory(relPath)
}

func (m *WorkingMemory) List() []string {
	files, err := m.Store.ListMemoryFiles()
	if err != nil {
		return []string{}
	}
	return files
}

func (m *WorkingMemory) FormatContext() string {
	ctxStr, _, err := m.Store.GetWorkingMemory()
	if err != nil {
		return "Error retrieving memory: " + err.Error()
	}
	return ctxStr
}
