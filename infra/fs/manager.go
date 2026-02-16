package fs

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// WorkspaceManager handles the lifecycle of execution workspaces and transactions.
type WorkspaceManager struct {
	ProjectRoot string
}

// NewWorkspaceManager initializes a manager with an absolute project root.
func NewWorkspaceManager(root string) *WorkspaceManager {
	absRoot, _ := filepath.Abs(root)
	return &WorkspaceManager{ProjectRoot: absRoot}
}

// --- Transaction System ---

// Transaction tracks changes for a single agent session to allow rollbacks.
type Transaction struct {
	backupDir    string
	projectRoot  string
	changedFiles []string
	newFiles     []string
	// backedUpPaths ensures we only save the ORIGINAL version once per transaction
	backedUpPaths map[string]bool

	// Link to shadow manager for cleanup
	shadow    *ShadowManager
	Committed bool
}

// BeginTransaction initializes a backup area for the current operation.
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

// AttachShadow links a ShadowManager to this transaction so it can be cleared on rollback.
func (t *Transaction) AttachShadow(s *ShadowManager) {
	t.shadow = s
}

// BackupFile creates a backup of a single file before modification.
func (t *Transaction) BackupFile(relPath string) error {
	// Idempotency Check: If we already backed this up, don't do it again.
	// We want the state of the file BEFORE the transaction started.
	if t.backedUpPaths[relPath] {
		return nil
	}

	srcPath := filepath.Join(t.projectRoot, relPath)
	dstPath := filepath.Join(t.backupDir, relPath)

	// Check if file exists
	info, err := os.Stat(srcPath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet - track it as new for deletion on rollback
			t.newFiles = append(t.newFiles, relPath)
			t.backedUpPaths[relPath] = true
			return nil
		}
		return fmt.Errorf("stat failed for %s: %w", relPath, err)
	}

	if info.IsDir() {
		return fmt.Errorf("cannot backup directory as file: %s", relPath)
	}

	// Create backup directory structure
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return fmt.Errorf("failed to create backup dir: %w", err)
	}

	// Copy file to backup
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

// Rollback restores all backed-up files, removes new files, AND clears shadow.
// It uses an atomic swap strategy to prevent data corruption during restoration.
func (t *Transaction) Rollback() error {
	if t.Committed {
		return nil
	}

	var errorList []string

	// 1. Restore modified files (Atomic Strategy)
	for _, relPath := range t.changedFiles {
		backupPath := filepath.Join(t.backupDir, relPath)
		realPath := filepath.Join(t.projectRoot, relPath)

		// Read the backup content
		data, err := os.ReadFile(backupPath)
		if err != nil {
			errorList = append(errorList, fmt.Sprintf("failed to read backup for %s: %v", relPath, err))
			continue
		}

		// Write to a temporary file first
		tmpPath := realPath + ".tmp"
		if err := os.WriteFile(tmpPath, data, 0644); err != nil {
			errorList = append(errorList, fmt.Sprintf("failed to write tmp file for %s: %v", relPath, err))
			continue
		}

		// Atomic Move: Rename tmp to real
		if err := os.Rename(tmpPath, realPath); err != nil {
			// Try to clean up the tmp file if rename failed
			_ = os.Remove(tmpPath)
			errorList = append(errorList, fmt.Sprintf("failed to restore %s: %v", relPath, err))
		}
	}

	// 2. Delete files that were created new
	for _, newFile := range t.newFiles {
		fullPath := filepath.Join(t.projectRoot, newFile)
		if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
			errorList = append(errorList, fmt.Sprintf("failed to remove new file %s: %v", newFile, err))
		}
	}

	// 3. Clear the shadow directory if attached
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

// Commit marks the transaction as successful.
func (t *Transaction) Commit() error {
	t.Committed = true
	// On success, we also want to clear shadow to ensure a clean slate for next run,
	// assuming all shadow files were applied via incremental commits.
	if t.shadow != nil {
		return t.shadow.Clear()
	}
	return nil
}
