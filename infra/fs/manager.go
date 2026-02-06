package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// WorkspaceManager handles the lifecycle of execution workspaces and audit trails
type WorkspaceManager struct {
	ProjectRoot string
}

// RunWorkspace represents the isolated directory for a specific execution
type RunWorkspace struct {
	ID      string
	DirPath string
}

func NewWorkspaceManager(root string) *WorkspaceManager {
	return &WorkspaceManager{ProjectRoot: root}
}

// CreateRunWorkspace initializes the directory structure: .codepicker/runs/<timestamp>
func (m *WorkspaceManager) CreateRunWorkspace() (*RunWorkspace, error) {
	// FIX: Corrected time layout from "2006-01-28" to "2006-01-02" so the day is accurate
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

// SaveArtifact writes a file (like context.txt, policy.json) to the run workspace
func (w *RunWorkspace) SaveArtifact(filename string, content []byte) error {
	path := filepath.Join(w.DirPath, filename)
	return os.WriteFile(path, content, 0644)
}

// Path returns the full path to a file within this workspace
func (w *RunWorkspace) Path(filename string) string {
	return filepath.Join(w.DirPath, filename)
}

// ListExecutions returns a list of past run directories (New helper for dashboarding)
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
