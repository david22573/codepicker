package agent

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/david22573/codepicker/internal/database"
	"github.com/david22573/codepicker/internal/shadow" // Import shadow
)

type WorkingMemory struct {
	SrcRoot string
	Store   *database.Store
	Shadow  *shadow.Manager // Add Shadow Manager
}

// Update constructor to accept shadow manager
func NewMemory(srcRoot string, store *database.Store, sm *shadow.Manager) *WorkingMemory {
	return &WorkingMemory{
		SrcRoot: srcRoot,
		Store:   store,
		Shadow:  sm,
	}
}

func (m *WorkingMemory) Add(relPath string) error {
	// 1. Try reading from Shadow Directory first (Overlay FS)
	// This allows the agent to "see" files it just created in previous steps
	var content []byte
	var err error

	shadowPath := m.Shadow.GetShadowPath(relPath)
	if _, statErr := os.Stat(shadowPath); statErr == nil {
		// File exists in shadow, use this version
		content, err = os.ReadFile(shadowPath)
	} else {
		// 2. Fallback to Source Directory
		fullPath := filepath.Join(m.SrcRoot, relPath)
		content, err = os.ReadFile(fullPath)
	}

	if err != nil {
		return fmt.Errorf("failed to read file (checked shadow & source): %w", err)
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

func (m *WorkingMemory) AddNote(content string) error {
	// We treat notes as system messages in the history
	return m.Store.AddMessage("system", content)
}
