package fs

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	Committed    bool // Exported so Agent can check status
}

// BeginTransaction initializes a backup area for the current operation.
func (m *WorkspaceManager) BeginTransaction() (*Transaction, error) {
	timestamp := time.Now().Format("20060102-150405")
	backupDir := filepath.Join(m.ProjectRoot, ".codepicker", "backups", timestamp)

	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create backup dir: %w", err)
	}

	return &Transaction{
		backupDir:    backupDir,
		projectRoot:  m.ProjectRoot,
		changedFiles: []string{},
		Committed:    false,
	}, nil
}

// BackupFile copies the current version of a file to the backup directory.
func (t *Transaction) BackupFile(relPath string) error {
	srcPath := filepath.Join(t.projectRoot, relPath)
	dstPath := filepath.Join(t.backupDir, relPath)

	// If file doesn't exist (new file), track for potential deletion
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		t.changedFiles = append(t.changedFiles, relPath)
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return err
	}

	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	if err == nil {
		t.changedFiles = append(t.changedFiles, relPath)
	}
	return err
}

// Rollback restores all backed-up files and removes new files.
func (t *Transaction) Rollback() error {
	if t.Committed {
		return nil
	}

	for _, relPath := range t.changedFiles {
		backupPath := filepath.Join(t.backupDir, relPath)
		realPath := filepath.Join(t.projectRoot, relPath)

		// If no backup exists, it was a brand new file; delete it
		if _, err := os.Stat(backupPath); os.IsNotExist(err) {
			os.Remove(realPath)
			continue
		}

		// Restore original content
		input, err := os.ReadFile(backupPath)
		if err == nil {
			os.WriteFile(realPath, input, 0644)
		}
	}
	return nil
}

// Commit marks the transaction as successful.
func (t *Transaction) Commit() {
	t.Committed = true
}

// --- Workspace Management ---

type RunWorkspace struct {
	ID      string
	DirPath string
}

func (m *WorkspaceManager) CreateRunWorkspace() (*RunWorkspace, error) {
	timestamp := time.Now().Format("2006-01-02T15-04-05")
	runDirName := timestamp

	fullPath := filepath.Join(m.ProjectRoot, ".codepicker", "runs", runDirName)

	if err := os.MkdirAll(fullPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create run workspace: %w", err)
	}

	return &RunWorkspace{
		ID:      runDirName,
		DirPath: fullPath,
	}, nil
}

func (m *WorkspaceManager) ListExecutions() ([]string, error) {
	runsDir := filepath.Join(m.ProjectRoot, ".codepicker", "runs")
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var runs []string
	for _, e := range entries {
		if e.IsDir() {
			runs = append(runs, e.Name())
		}
	}
	return runs, nil
}
