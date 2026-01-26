package fs

import (
	"os"
	"path/filepath"

	"github.com/david22573/codepicker/domain/errors"
)

const ShadowDir = ".codepicker/shadow"

type ShadowManager struct {
	ProjectRoot string
}

func NewShadowManager(root string) *ShadowManager {
	return &ShadowManager{ProjectRoot: root}
}

// Write saves content to the shadow directory, mirroring the structure
func (s *ShadowManager) Write(relPath string, content []byte) (string, error) {
	shadowPath := filepath.Join(s.ProjectRoot, ShadowDir, relPath)

	if err := os.MkdirAll(filepath.Dir(shadowPath), 0755); err != nil {
		return "", errors.NewSystem("fs.Write", "failed to create shadow dirs", err)
	}

	if err := os.WriteFile(shadowPath, content, 0644); err != nil {
		return "", errors.NewSystem("fs.Write", "failed to write shadow file", err)
	}

	return shadowPath, nil
}

// Read tries to read from shadow first, then falls back to real FS
func (s *ShadowManager) Read(relPath string) ([]byte, error) {
	shadowPath := filepath.Join(s.ProjectRoot, ShadowDir, relPath)
	realPath := filepath.Join(s.ProjectRoot, relPath)

	// Try shadow first
	if _, err := os.Stat(shadowPath); err == nil {
		return os.ReadFile(shadowPath)
	}

	// Fallback to real
	content, err := os.ReadFile(realPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.NewValidation("fs.Read", "file not found: "+relPath)
		}
		return nil, errors.NewSystem("fs.Read", "io error", err)
	}
	return content, nil
}

// Apply moves a file from shadow to real FS (The "Commit" action)
func (s *ShadowManager) Apply(relPath string) error {
	shadowPath := filepath.Join(s.ProjectRoot, ShadowDir, relPath)
	realPath := filepath.Join(s.ProjectRoot, relPath)

	content, err := os.ReadFile(shadowPath)
	if err != nil {
		return errors.NewValidation("fs.Apply", "shadow file not found or unreadable")
	}

	if err := os.MkdirAll(filepath.Dir(realPath), 0755); err != nil {
		return errors.NewSystem("fs.Apply", "failed to create dirs", err)
	}

	if err := os.WriteFile(realPath, content, 0644); err != nil {
		return errors.NewSystem("fs.Apply", "failed to write real file", err)
	}

	// Clean up shadow
	os.Remove(shadowPath)
	return nil
}
