package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/writer"
	ignore "github.com/sabhiram/go-gitignore"
)

type Scanner struct {
	Root      string
	Writer    writer.OutputStrategy
	Config    *config.Config
	GitIgnore *ignore.GitIgnore
}

func NewScanner(root string, w writer.OutputStrategy, cfg *config.Config) *Scanner {
	s := &Scanner{
		Root:   root,
		Writer: w,
		Config: cfg,
	}

	// Attempt to load .gitignore
	gitIgnorePath := filepath.Join(root, ".gitignore")
	if _, err := os.Stat(gitIgnorePath); err == nil {
		ign, _ := ignore.CompileIgnoreFile(gitIgnorePath)
		s.GitIgnore = ign
	}

	return s
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

		// Calculate relative path for matching
		relPath, _ := filepath.Rel(s.Root, path)
		if relPath == "." {
			return nil
		}

		// 1. Check .gitignore
		if s.GitIgnore != nil && s.GitIgnore.MatchesPath(relPath) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// 2. Ask Writer if we should skip (e.g., output file)
		if s.Writer.ShouldSkip(path) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// 3. Ignore Directories (using Config)
		if d.IsDir() {
			// Skip hidden dirs unless specifically allowed?
			// Usually hidden dirs (.git) are good to skip by default.
			if strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
				return filepath.SkipDir
			}
			if s.Config.IgnoredDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		// 4. Filter Files (using Config)
		ext := strings.ToLower(filepath.Ext(path))
		name := strings.ToLower(d.Name())

		if !s.Config.AllowedExts[ext] && !config.IsSpecialFile(name) {
			return nil
		}

		// 5. Write
		// Feedback: only print if not Tree strategy (handled by Strategy now via Name check if desired,
		// but simple log here is fine too)
		if s.Writer.Name() != "Tree" {
			fmt.Printf("   Picked: %s\n", relPath)
		}

		return s.Writer.Write(path, relPath)
	})
}
