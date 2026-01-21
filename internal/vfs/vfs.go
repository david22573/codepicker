package vfs

import (
	"os"
	"path/filepath"

	"github.com/david22573/codepicker/internal/shadow"
)

type VirtualFileSystem interface {
	ReadFile(relPath string) ([]byte, error)

	WriteFile(relPath string, content []byte) (string, error)
}

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

	shadowPath := fs.Shadow.GetShadowPath(relPath)
	if _, err := os.Stat(shadowPath); err == nil {
		return os.ReadFile(shadowPath)
	}

	fullPath := filepath.Join(fs.SrcRoot, relPath)
	return os.ReadFile(fullPath)
}

func (fs *OverlayFS) WriteFile(relPath string, content []byte) (string, error) {

	return fs.Shadow.WriteFile(relPath, content)
}

func (fs *OverlayFS) GetShadowManager() *shadow.Manager {
	return fs.Shadow
}
