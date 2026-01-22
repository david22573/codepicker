package agent

import (
	"fmt"

	"github.com/david22573/codepicker/internal/database"
	"github.com/david22573/codepicker/internal/vfs"
)

type WorkingMemory struct {
	Store *database.Store
	FS    vfs.VirtualFileSystem
	Trace bool
}

func NewMemory(store *database.Store, fs vfs.VirtualFileSystem, trace bool) *WorkingMemory {
	return &WorkingMemory{Store: store, FS: fs, Trace: trace}
}

func (m *WorkingMemory) Add(relPath string) error {
	if m.Trace {
		fmt.Printf("[MEMORY] + Adding: %s\n", relPath)
	}
	content, err := m.FS.ReadFile(relPath)
	if err != nil {
		return err
	}
	return m.Store.UpdateWorkingMemory(relPath, string(content))
}

func (m *WorkingMemory) Remove(relPath string) {
	if m.Trace {
		fmt.Printf("[MEMORY] - Removing: %s\n", relPath)
	}
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
	// Calls Store.GetWorkingMemory, which now has the "Smart Context" sorting
	// and the 100k token safety cap.
	ctxStr, tokens, err := m.Store.GetWorkingMemory()
	if err != nil {
		return "Error retrieving memory: " + err.Error()
	}
	if m.Trace {
		fmt.Printf("[MEMORY] Context: %d tokens\n", tokens)
	}
	return ctxStr
}

func (m *WorkingMemory) AddNote(content string) error {
	return m.Store.AddMessage("system", content)
}

func (m *WorkingMemory) Snapshot() (interface{}, error) {
	if m.Trace {
		fmt.Println("[MEMORY] 📸 Snapshotting...")
	}
	return m.Store.CreateSnapshot()
}

func (m *WorkingMemory) Restore(snap interface{}) error {
	if m.Trace {
		fmt.Println("[MEMORY] ⏪ Restoring...")
	}
	typedSnap, ok := snap.(*database.MemorySnapshot)
	if !ok {
		return fmt.Errorf("invalid snapshot type")
	}
	return m.Store.RestoreSnapshot(typedSnap)
}
