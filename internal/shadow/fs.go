package shadow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/david22573/codepicker/internal/paths"
)

const (
	ShadowDirName = ".codepicker/shadow"
	BackupDirName = ".codepicker/backups"
	ManifestName  = "manifest.json"
	MaxShadowSize = 1024 * 1024 * 1 // 1MB Limit
)

type ChangeMeta struct {
	File      string    `json:"file"`
	Agent     string    `json:"agent"`
	Task      string    `json:"task"`
	Timestamp time.Time `json:"timestamp"`
}

type Manifest struct {
	Changes map[string]ChangeMeta `json:"changes"`
}

type Manager struct {
	SrcRoot    string
	ShadowRoot string
	BackupRoot string
	Manifest   Manifest
}

func NewManager(srcRoot string) (*Manager, error) {
	absSrc, err := paths.Sanitize(srcRoot)
	if err != nil {
		return nil, err
	}

	shadowRoot := filepath.Join(absSrc, ShadowDirName)
	backupRoot := filepath.Join(absSrc, BackupDirName)

	if err := os.MkdirAll(shadowRoot, 0755); err != nil {
		return nil, fmt.Errorf("failed to create shadow root: %w", err)
	}

	m := &Manager{
		SrcRoot:    absSrc,
		ShadowRoot: shadowRoot,
		BackupRoot: backupRoot,
		Manifest:   Manifest{Changes: make(map[string]ChangeMeta)},
	}
	m.LoadManifest()

	return m, nil
}

// WriteFile writes content to the shadow directory
func (m *Manager) WriteFile(relPath string, content []byte) (string, error) {
	cleanRel := filepath.Clean(relPath)
	if strings.HasPrefix(cleanRel, "..") || strings.HasPrefix(cleanRel, "/") {
		return "", fmt.Errorf("invalid path: must be relative")
	}

	if len(content) > MaxShadowSize {
		return "", fmt.Errorf("content exceeds max shadow file size (%d bytes)", MaxShadowSize)
	}

	shadowPath := filepath.Join(m.ShadowRoot, cleanRel)

	if !strings.HasPrefix(shadowPath, m.ShadowRoot) {
		return "", fmt.Errorf("security violation: attempted to write outside shadow directory")
	}

	if err := os.MkdirAll(filepath.Dir(shadowPath), 0755); err != nil {
		return "", err
	}

	if err := os.WriteFile(shadowPath, content, 0644); err != nil {
		return "", err
	}

	return shadowPath, nil
}

func (m *Manager) RecordAttribution(relPath, agent, task string) error {
	m.Manifest.Changes[relPath] = ChangeMeta{
		File:      relPath,
		Agent:     agent,
		Task:      task,
		Timestamp: time.Now(),
	}
	return m.saveManifest()
}

func (m *Manager) LoadManifest() {
	path := filepath.Join(m.ShadowRoot, ManifestName)
	data, err := os.ReadFile(path)
	if err == nil {
		json.Unmarshal(data, &m.Manifest)
	}
	if m.Manifest.Changes == nil {
		m.Manifest.Changes = make(map[string]ChangeMeta)
	}
}

func (m *Manager) saveManifest() error {
	path := filepath.Join(m.ShadowRoot, ManifestName)
	data, _ := json.MarshalIndent(m.Manifest, "", "  ")
	return os.WriteFile(path, data, 0644)
}

func (m *Manager) GetShadowPath(relPath string) string {
	return filepath.Join(m.ShadowRoot, relPath)
}

// ApplyAtomic applies a single file but creates a backup first.
// Returns the path to the backup file if successful.
func (m *Manager) ApplyAtomic(relPath string) (string, error) {
	shadowPath := filepath.Join(m.ShadowRoot, relPath)
	realPath := filepath.Join(m.SrcRoot, relPath)

	content, err := os.ReadFile(shadowPath)
	if err != nil {
		return "", fmt.Errorf("shadow file not found: %w", err)
	}

	// 1. Create Backup
	backupPath, err := m.createBackup(relPath)
	if err != nil {
		return "", fmt.Errorf("failed to create safety backup: %w", err)
	}

	// 2. Overwrite Destination
	if err := os.MkdirAll(filepath.Dir(realPath), 0755); err != nil {
		return backupPath, err
	}

	if err := os.WriteFile(realPath, content, 0644); err != nil {
		return backupPath, err
	}

	return backupPath, nil
}

// createBackup copies the current live file to .codepicker/backups/timestamp/file
func (m *Manager) createBackup(relPath string) (string, error) {
	realPath := filepath.Join(m.SrcRoot, relPath)

	// If file doesn't exist, nothing to back up (it's a new file)
	if _, err := os.Stat(realPath); os.IsNotExist(err) {
		return "", nil
	}

	ts := time.Now().Format("20060102-150405")
	backupPath := filepath.Join(m.BackupRoot, ts, relPath)

	if err := os.MkdirAll(filepath.Dir(backupPath), 0755); err != nil {
		return "", err
	}

	src, err := os.Open(realPath)
	if err != nil {
		return "", err
	}
	defer src.Close()

	dst, err := os.Create(backupPath)
	if err != nil {
		return "", err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", err
	}

	return backupPath, nil
}

// Restore copies the backup file back to the real path
func (m *Manager) Restore(relPath, backupPath string) error {
	if backupPath == "" {
		// If no backup path, it means the file was new. We should delete the created file.
		realPath := filepath.Join(m.SrcRoot, relPath)
		return os.Remove(realPath)
	}

	realPath := filepath.Join(m.SrcRoot, relPath)
	src, err := os.Open(backupPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(realPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

func (m *Manager) Cleanup() error {
	return os.RemoveAll(m.ShadowRoot)
}

func (m *Manager) ListShadowFiles() ([]string, error) {
	var files []string
	if _, err := os.Stat(m.ShadowRoot); os.IsNotExist(err) {
		return files, nil
	}

	err := filepath.WalkDir(m.ShadowRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() == ManifestName {
			return nil
		}
		rel, err := filepath.Rel(m.ShadowRoot, path)
		if err != nil {
			return err
		}
		files = append(files, rel)
		return nil
	})
	return files, err
}

// PreviewDiff generates a diff. Falls back to simple text comparison if system diff fails.
func (m *Manager) PreviewDiff(relPath string) (string, error) {
	shadowPath := filepath.Join(m.ShadowRoot, relPath)
	realPath := filepath.Join(m.SrcRoot, relPath)

	shadowBytes, err := os.ReadFile(shadowPath)
	if err != nil {
		return "", fmt.Errorf("reading shadow: %w", err)
	}

	realBytes, err := os.ReadFile(realPath)
	if os.IsNotExist(err) {
		return fmt.Sprintf("+++ NEW FILE: %s\n\n%s", relPath, string(shadowBytes)), nil
	}
	if err != nil {
		return "", fmt.Errorf("reading source: %w", err)
	}

	if bytes.Equal(shadowBytes, realBytes) {
		return fmt.Sprintf("=== %s (No changes)", relPath), nil
	}

	// Try system diff
	cmd := exec.Command("diff", "-u", "--color=always", realPath, shadowPath)
	out, err := cmd.CombinedOutput()
	if len(out) > 0 {
		return string(out), nil
	}

	// Fallback for Windows/Container without diff
	return fmt.Sprintf("MODIFIED: %s\n<<< OLD (%d bytes)\n%s\n>>> NEW (%d bytes)\n%s",
		relPath,
		len(realBytes), string(realBytes),
		len(shadowBytes), string(shadowBytes),
	), nil
}
