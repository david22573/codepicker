package agent

import (
	"fmt"

	"github.com/david22573/codepicker/internal/database"
	"github.com/david22573/codepicker/internal/vfs"
)

type WorkingMemory struct {
	Store *database.Store
	FS    vfs.VirtualFileSystem
}

func NewMemory(store *database.Store, fs vfs.VirtualFileSystem) *WorkingMemory {
	return &WorkingMemory{
		Store: store,
		FS:    fs,
	}
}

func (m *WorkingMemory) Add(relPath string) error {
	content, err := m.FS.ReadFile(relPath)
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

func (m *WorkingMemory) AddNote(content string) error {
	return m.Store.AddMessage("system", content)
}

// Snapshot Wrapper
func (m *WorkingMemory) Snapshot() (interface{}, error) {
	return m.Store.CreateSnapshot()
}

// Restore Wrapper - Fixed signature to match interface{}
func (m *WorkingMemory) Restore(snap interface{}) error {
	typedSnap, ok := snap.(*database.MemorySnapshot)
	if !ok {
		return fmt.Errorf("invalid snapshot type")
	}
	return m.Store.RestoreSnapshot(typedSnap)
}
