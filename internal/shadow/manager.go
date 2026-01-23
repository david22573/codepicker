package shadow

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

type ChangeMetadata struct {
	Hash         string    `json:"hash"`
	Timestamp    time.Time `json:"timestamp"`
	Agent        string    `json:"agent"`
	OriginalPath string    `json:"original_path"`
}

type Manifest struct {
	Version string                    `json:"version"`
	Changes map[string]ChangeMetadata `json:"changes"` // Key is relative path
}

type Manager struct {
	ShadowRoot   string
	srcRoot      string
	manifestPath string
	Manifest     *Manifest
	mu           sync.RWMutex
}

func NewManager(srcRoot string) (*Manager, error) {
	shadowRoot := filepath.Join(srcRoot, ".codepicker", "shadow")
	if err := os.MkdirAll(shadowRoot, 0755); err != nil {
		return nil, fmt.Errorf("failed to create shadow root: %w", err)
	}

	m := &Manager{
		ShadowRoot:   shadowRoot,
		srcRoot:      srcRoot,
		manifestPath: filepath.Join(shadowRoot, "manifest.json"),
		Manifest: &Manifest{
			Version: "1.0",
			Changes: make(map[string]ChangeMetadata),
		},
	}

	// Try to load existing manifest, ignore error if missing
	_ = m.LoadManifest()
	return m, nil
}

func (m *Manager) SaveManifest() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := json.MarshalIndent(m.Manifest, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(m.manifestPath, data, 0644)
}

func (m *Manager) LoadManifest() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.manifestPath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, m.Manifest)
}

func (m *Manager) GetManifestChanges() map[string]ChangeMetadata {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// Return a copy to prevent race conditions during iteration
	copy := make(map[string]ChangeMetadata)
	for k, v := range m.Manifest.Changes {
		copy[k] = v
	}
	return copy
}

func (m *Manager) GetShadowPath(relPath string) string {
	return filepath.Join(m.ShadowRoot, relPath)
}

func (m *Manager) WriteFile(relPath string, content []byte) (string, error) {
	shadowPath := m.GetShadowPath(relPath)

	if err := os.MkdirAll(filepath.Dir(shadowPath), 0755); err != nil {
		return "", err
	}

	if err := os.WriteFile(shadowPath, content, 0644); err != nil {
		return "", err
	}

	// Update Manifest
	m.mu.Lock()
	m.Manifest.Changes[relPath] = ChangeMetadata{
		Hash:         fmt.Sprintf("size:%d", len(content)), // Simplified hash
		Timestamp:    time.Now(),
		Agent:        "codepicker",
		OriginalPath: relPath,
	}
	m.mu.Unlock()
	m.SaveManifest()

	return shadowPath, nil
}

func (m *Manager) ListShadowFiles() ([]string, error) {
	var files []string
	err := filepath.Walk(m.ShadowRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Name() == "manifest.json" {
			return nil
		}
		rel, _ := filepath.Rel(m.ShadowRoot, path)
		files = append(files, rel)
		return nil
	})
	return files, err
}

func (m *Manager) ApplyAtomic(relPath string) (string, error) {
	shadowPath := m.GetShadowPath(relPath)
	destPath := filepath.Join(m.srcRoot, relPath)

	// Create backup
	backupPath := destPath + ".bak"
	if _, err := os.Stat(destPath); err == nil {
		// File exists, copy to backup
		input, err := os.ReadFile(destPath)
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(backupPath, input, 0644); err != nil {
			return "", err
		}
	}

	// Move shadow to dest
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return "", err
	}

	input, err := os.ReadFile(shadowPath)
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(destPath, input, 0644); err != nil {
		return "", err
	}

	// Remove from manifest and delete shadow file
	m.mu.Lock()
	delete(m.Manifest.Changes, relPath)
	m.mu.Unlock()
	m.SaveManifest()
	os.Remove(shadowPath)

	return backupPath, nil
}

func (m *Manager) Restore(relPath, backupPath string) error {
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return nil // Nothing to restore
	}

	destPath := filepath.Join(m.srcRoot, relPath)
	input, err := os.ReadFile(backupPath)
	if err != nil {
		return err
	}

	if err := os.WriteFile(destPath, input, 0644); err != nil {
		return err
	}

	return os.Remove(backupPath)
}

func (m *Manager) Cleanup() error {
	return os.RemoveAll(m.ShadowRoot)
}

func (m *Manager) PreviewDiff(relPath string) (string, error) {
	shadowPath := m.GetShadowPath(relPath)
	destPath := filepath.Join(m.srcRoot, relPath)

	// If destination doesn't exist, it's a new file
	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		return fmt.Sprintf("New File: %s", relPath), nil
	}

	// Use git diff if available, otherwise simple comparison
	cmd := exec.Command("diff", "-u", "--color=always", destPath, shadowPath)
	out, err := cmd.CombinedOutput()

	// diff returns exit code 1 if differences found, which is what we want
	if err != nil && len(out) > 0 {
		return string(out), nil
	}
	if len(out) == 0 {
		return "No changes detected", nil
	}
	return string(out), nil
}
