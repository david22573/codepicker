package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type FileSnapshot struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Tokens  int    `json:"tokens"` // Placeholder for future token counting
}

type WorkingMemory struct {
	SrcRoot string
	Files   map[string]FileSnapshot
	mu      sync.RWMutex
}

func NewMemory(srcRoot string) *WorkingMemory {
	return &WorkingMemory{
		SrcRoot: srcRoot,
		Files:   make(map[string]FileSnapshot),
	}
}

// Add reads a file from disk and adds/updates it in memory
func (m *WorkingMemory) Add(relPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	fullPath := filepath.Join(m.SrcRoot, relPath)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	m.Files[relPath] = FileSnapshot{
		Path:    relPath,
		Content: string(content),
		Tokens:  len(content) / 4, // Rough estimate
	}
	return nil
}

// Remove deletes a file from memory
func (m *WorkingMemory) Remove(relPath string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Files, relPath)
}

// List returns currently tracked files
func (m *WorkingMemory) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keys := make([]string, 0, len(m.Files))
	for k := range m.Files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// FormatContext generates the string to be injected into the System Prompt
func (m *WorkingMemory) FormatContext() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.Files) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n### ACTIVE SOURCE FILES (READ-ONLY CONTEXT):\n")

	// Sort for deterministic prompt caching
	keys := m.List()
	for _, path := range keys {
		file := m.Files[path]
		sb.WriteString(fmt.Sprintf("--- BEGIN FILE: %s ---\n", path))
		sb.WriteString(file.Content)
		sb.WriteString(fmt.Sprintf("\n--- END FILE: %s ---\n\n", path))
	}

	return sb.String()
}
