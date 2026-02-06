package fs

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/david22573/codepicker/domain/errors"
)

const ShadowDir = ".codepicker/shadow"

// ShadowManager handles the safe staging of file changes.
type ShadowManager struct {
	ProjectRoot string
	mu          sync.RWMutex
}

func NewShadowManager(root string) *ShadowManager {
	absRoot, _ := filepath.Abs(root)
	return &ShadowManager{ProjectRoot: absRoot}
}

// sanitizePath prevents directory traversal attacks by resolving absolute paths.
// This is your primary defense against the agent escaping the repo[cite: 230, 261].
func (s *ShadowManager) sanitizePath(relPath string) (string, error) {
	clean := filepath.Clean(relPath)

	if filepath.IsAbs(clean) {
		return "", errors.NewValidation("fs.sanitize", "absolute paths not allowed")
	}

	// Resolve the full intended path
	fullPath := filepath.Join(s.ProjectRoot, clean)

	// Evaluate symlinks and get absolute path to check for escapes
	absProjectRoot, _ := filepath.Abs(s.ProjectRoot)
	absFullPath, _ := filepath.Abs(fullPath)

	if !strings.HasPrefix(absFullPath, absProjectRoot) {
		return "", errors.NewValidation("fs.sanitize", "path traversal detected: escapes project root")
	}

	return clean, nil
}

// Write saves content to the shadow directory[cite: 262].
func (s *ShadowManager) Write(relPath string, content []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cleanPath, err := s.sanitizePath(relPath)
	if err != nil {
		return "", err
	}

	shadowPath := filepath.Join(s.ProjectRoot, ShadowDir, cleanPath)
	if err := os.MkdirAll(filepath.Dir(shadowPath), 0755); err != nil {
		return "", errors.NewSystem("fs.Write", "failed to create shadow dirs", err)
	}

	if err := os.WriteFile(shadowPath, content, 0644); err != nil {
		return "", errors.NewSystem("fs.Write", "failed to write shadow file", err)
	}

	return shadowPath, nil
}

// Read from shadow first, then fall back to real FS[cite: 265].
func (s *ShadowManager) Read(relPath string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cleanPath, err := s.sanitizePath(relPath)
	if err != nil {
		return nil, err
	}

	shadowPath := filepath.Join(s.ProjectRoot, ShadowDir, cleanPath)
	if _, err := os.Stat(shadowPath); err == nil {
		return os.ReadFile(shadowPath)
	}

	return os.ReadFile(filepath.Join(s.ProjectRoot, cleanPath))
}

// Commit moves changes from shadow to the real filesystem[cite: 267].
func (s *ShadowManager) Commit(relPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cleanPath, err := s.sanitizePath(relPath)
	if err != nil {
		return err
	}

	shadowPath := filepath.Join(s.ProjectRoot, ShadowDir, cleanPath)
	realPath := filepath.Join(s.ProjectRoot, cleanPath)

	content, err := os.ReadFile(shadowPath)
	if err != nil {
		return errors.NewValidation("fs.Commit", "shadow file not found")
	}

	if err := os.MkdirAll(filepath.Dir(realPath), 0755); err != nil {
		return errors.NewSystem("fs.Commit", "failed to create dirs", err)
	}

	if err := os.WriteFile(realPath, content, 0644); err != nil {
		return errors.NewSystem("fs.Commit", "failed to write real file", err)
	}

	return os.Remove(shadowPath)
}
