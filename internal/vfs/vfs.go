package vfs

import (
	"os"
	"path/filepath"

	"github.com/david22573/codepicker/internal/shadow"
)

// VirtualFileSystem defines how the agent interacts with files.
// It abstracts the difference between the source code and the shadow workspace.
type VirtualFileSystem interface {
	// ReadFile returns the content of a file.
	// It should check the shadow filesystem first, then fall back to source.
	ReadFile(relPath string) ([]byte, error)

	// WriteFile writes content to the shadow filesystem.
	// It should NEVER write directly to source.
	WriteFile(relPath string, content []byte) (string, error)
}

// OverlayFS implements VirtualFileSystem by layering shadow over source.
type OverlayFS struct {
	SrcRoot string
	Shadow  *shadow.Manager
}

func NewOverlayFS(srcRoot string, sm *shadow.Manager) *OverlayFS {
	return &OverlayFS{
		SrcRoot: srcRoot,
		Shadow:  sm,
	}
}

func (fs *OverlayFS) ReadFile(relPath string) ([]byte, error) {
	// 1. Try to read from Shadow
	shadowPath := fs.Shadow.GetShadowPath(relPath)
	if _, err := os.Stat(shadowPath); err == nil {
		return os.ReadFile(shadowPath)
	}

	// 2. Fallback to Source
	fullPath := filepath.Join(fs.SrcRoot, relPath)
	return os.ReadFile(fullPath)
}

func (fs *OverlayFS) WriteFile(relPath string, content []byte) (string, error) {
	// Always write to shadow
	return fs.Shadow.WriteFile(relPath, content)
}
