package shadow

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/internal/paths"
)

const (
	ShadowDirName = ".codepicker/shadow"
	MaxShadowSize = 1024 * 1024 * 1 // 1MB Limit for shadow files
)

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

func (m *Manager) WriteFile(relPath string, content []byte) (string, error) {
	// 1. Sanitize the relative path to remove dotfiles/traversal logic
	// We treat the relPath as relative to CWD for sanitization purposes first
	// to ensure it doesn't contain traversal characters.
	cleanRel := filepath.Clean(relPath)
	if strings.HasPrefix(cleanRel, "..") || strings.HasPrefix(cleanRel, "/") {
		return "", fmt.Errorf("invalid path: must be relative")
	}

	// 2. Size Check
	if len(content) > MaxShadowSize {
		return "", fmt.Errorf("content exceeds max shadow file size (%d bytes)", MaxShadowSize)
	}

	// 3. Construct full path
	shadowPath := filepath.Join(m.ShadowRoot, cleanRel)

	// 4. Security Check: Ensure the resulting path is actually inside ShadowRoot
	// This prevents symlink attacks or weird join behaviors
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

func (m *Manager) GetShadowPath(relPath string) string {
	return filepath.Join(m.ShadowRoot, relPath)
}

func (m *Manager) Apply(relPath string) error {
	shadowPath := filepath.Join(m.ShadowRoot, relPath)
	realPath := filepath.Join(m.SrcRoot, relPath)

	content, err := os.ReadFile(shadowPath)
	if err != nil {
		return fmt.Errorf("shadow file not found: %w", err)
	}

	return os.WriteFile(realPath, content, 0644)
}

func (m *Manager) Cleanup() error {
	return os.RemoveAll(m.ShadowRoot)
}

func (m *Manager) ListShadowFiles() ([]string, error) {
	var files []string
	err := filepath.WalkDir(m.ShadowRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
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

func (m *Manager) PreviewDiff(relPath string) (string, error) {
	shadowPath := filepath.Join(m.ShadowRoot, relPath)
	realPath := filepath.Join(m.SrcRoot, relPath)

	shadowContent, err := os.ReadFile(shadowPath)
	if err != nil {
		return "", fmt.Errorf("could not read shadow file: %w", err)
	}

	realContent, err := os.ReadFile(realPath)
	if os.IsNotExist(err) {
		return fmt.Sprintf("+++ NEW FILE: %s\n(File does not exist in source)\n", relPath), nil
	} else if err != nil {
		return "", fmt.Errorf("could not read source file: %w", err)
	}

	if string(shadowContent) == string(realContent) {
		return fmt.Sprintf("=== %s\n(No changes detected)", relPath), nil
	}

	return fmt.Sprintf("M   %s\n    Original Size: %d bytes\n    New Size:      %d bytes",
		relPath, len(realContent), len(shadowContent)), nil
}
