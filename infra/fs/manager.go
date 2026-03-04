package fs

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type WorkspaceManager struct {
	ProjectRoot string
}

func NewWorkspaceManager(root string) *WorkspaceManager {
	absRoot, _ := filepath.Abs(root)
	return &WorkspaceManager{ProjectRoot: absRoot}
}

type Transaction struct {
	mu            sync.Mutex
	backupDir     string
	projectRoot   string
	changedFiles  []string
	newFiles      []string
	backedUpPaths map[string]bool
	shadow        *ShadowManager
	Committed     bool
}

func (m *WorkspaceManager) BeginTransaction() (*Transaction, error) {
	timestamp := time.Now().Format("20060102-150405")
	backupDir := filepath.Join(m.ProjectRoot, ".codepicker", "backups", timestamp)

	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create backup dir: %w", err)
	}

	return &Transaction{
		backupDir:     backupDir,
		projectRoot:   m.ProjectRoot,
		changedFiles:  []string{},
		newFiles:      []string{},
		backedUpPaths: make(map[string]bool),
		Committed:     false,
	}, nil
}

func (t *Transaction) AttachShadow(s *ShadowManager) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.shadow = s
}

func (t *Transaction) BackupFile(relPath string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.backedUpPaths[relPath] {
		return nil
	}

	srcPath := filepath.Join(t.projectRoot, relPath)
	dstPath := filepath.Join(t.backupDir, relPath)

	info, err := os.Stat(srcPath)
	if err != nil {
		if os.IsNotExist(err) {
			t.newFiles = append(t.newFiles, relPath)
			t.backedUpPaths[relPath] = true
			return nil
		}
		return fmt.Errorf("stat failed for %s: %w", relPath, err)
	}

	if info.IsDir() {
		return fmt.Errorf("cannot backup directory as file: %s", relPath)
	}

	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return fmt.Errorf("failed to create backup dir: %w", err)
	}

	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open source: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("backup copy failed: %w", err)
	}

	t.changedFiles = append(t.changedFiles, relPath)
	t.backedUpPaths[relPath] = true
	return nil
}

func (t *Transaction) Rollback() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.Committed {
		return nil
	}

	var errorList []string

	for _, relPath := range t.changedFiles {
		backupPath := filepath.Join(t.backupDir, relPath)
		realPath := filepath.Join(t.projectRoot, relPath)

		data, err := os.ReadFile(backupPath)
		if err != nil {
			errorList = append(errorList, fmt.Sprintf("failed to read backup for %s: %v", relPath, err))
			continue
		}

		tmpPath := realPath + ".tmp"
		if err := os.WriteFile(tmpPath, data, 0644); err != nil {
			errorList = append(errorList, fmt.Sprintf("failed to write tmp file for %s: %v", relPath, err))
			continue
		}

		if err := os.Rename(tmpPath, realPath); err != nil {
			if copyErr := copyFile(tmpPath, realPath); copyErr != nil {
				_ = os.Remove(tmpPath)
				errorList = append(errorList, fmt.Sprintf("failed to restore %s: rename err: %v, copy err: %v", relPath, err, copyErr))
			} else {
				_ = os.Remove(tmpPath)
			}
		}
	}

	for _, newFile := range t.newFiles {
		fullPath := filepath.Join(t.projectRoot, newFile)
		if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
			errorList = append(errorList, fmt.Sprintf("failed to remove new file %s: %v", newFile, err))
		}
	}

	if t.shadow != nil {
		if err := t.shadow.Clear(); err != nil {
			errorList = append(errorList, fmt.Sprintf("failed to clear shadow: %v", err))
		}
	}

	if len(errorList) > 0 {
		return fmt.Errorf("rollback completed with errors:\n- %s", strings.Join(errorList, "\n- "))
	}
	return nil
}

func (t *Transaction) Commit() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.Committed = true
	if t.shadow != nil {
		return t.shadow.Clear()
	}
	return nil
}
