package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/writer"
)

type Scanner struct {
	Root   string
	Writer writer.OutputStrategy
}

func NewScanner(root string, w writer.OutputStrategy) *Scanner {
	return &Scanner{Root: root, Writer: w}
}

func (s *Scanner) Scan() error {
	if err := s.Writer.Init(); err != nil {
		return fmt.Errorf("writer init failed: %w", err)
	}
	defer s.Writer.Close()

	return filepath.WalkDir(s.Root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// 1. Ask Writer if we should skip
		if s.Writer.ShouldSkip(path) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// 2. Ignore Directories (using Config)
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
				return filepath.SkipDir
			}
			if config.IgnoredDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		// 3. Filter Files (using Config)
		ext := strings.ToLower(filepath.Ext(path))
		name := strings.ToLower(d.Name())

		if !config.AllowedExts[ext] && !config.IsSpecialFile(name) {
			return nil
		}

		// 4. Write
		relPath, _ := filepath.Rel(s.Root, path)
		return s.Writer.Write(path, relPath)
	})
}

