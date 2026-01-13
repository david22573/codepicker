package shadow

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/internal/paths"
)

const ShadowDirName = ".codepicker/shadow"

type Manager struct {
	SrcRoot    string
	ShadowRoot string
}

func NewManager(srcRoot string) (*Manager, error) {
	absSrc, err := paths.Sanitize(srcRoot)
	if err != nil {
		return nil, err
	}

	shadowRoot := filepath.Join(absSrc, ShadowDirName)
	if err := os.MkdirAll(shadowRoot, 0755); err != nil {
		return nil, fmt.Errorf("failed to create shadow root: %w", err)
	}

	return &Manager{
		SrcRoot:    absSrc,
		ShadowRoot: shadowRoot,
	}, nil
}

// WriteFile writes content to the shadow directory, mirroring the structure of the real project.
// It returns the absolute path of the written shadow file.
func (m *Manager) WriteFile(relPath string, content []byte) (string, error) {
	// 1. Sanity check: Ensure relPath doesn't escape
	if strings.Contains(relPath, "..") {
		return "", fmt.Errorf("invalid path: cannot escape project root")
	}

	// 2. Construct shadow path
	shadowPath := filepath.Join(m.ShadowRoot, relPath)

	// 3. Ensure parent directories exist in shadow
	if err := os.MkdirAll(filepath.Dir(shadowPath), 0755); err != nil {
		return "", err
	}

	// 4. Write the content
	if err := os.WriteFile(shadowPath, content, 0644); err != nil {
		return "", err
	}

	return shadowPath, nil
}

// GetDiff returns the diff between the real file and the shadow file
// This is useful for the Neovim client to display changes.
// Returns empty string if no diff or file doesn't exist.
func (m *Manager) GetShadowPath(relPath string) string {
	return filepath.Join(m.ShadowRoot, relPath)
}

// Apply accepts the shadow change and overwrites the real file
func (m *Manager) Apply(relPath string) error {
	shadowPath := filepath.Join(m.ShadowRoot, relPath)
	realPath := filepath.Join(m.SrcRoot, relPath)

	content, err := os.ReadFile(shadowPath)
	if err != nil {
		return fmt.Errorf("shadow file not found: %w", err)
	}

	return os.WriteFile(realPath, content, 0644)
}
