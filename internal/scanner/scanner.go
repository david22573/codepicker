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
	Root         string
	Writer       writer.OutputStrategy
	Config       *config.Config
	GitIgnore    *ignore.GitIgnore
	CustomIgnore *ignore.GitIgnore // Add field for our custom ignore file
}

func NewScanner(root string, w writer.OutputStrategy, cfg *config.Config) *Scanner {
	s := &Scanner{
		Root:   root,
		Writer: w,
		Config: cfg,
	}

	// 1. Load .gitignore if it exists
	gitIgnorePath := filepath.Join(root, ".gitignore")
	if _, err := os.Stat(gitIgnorePath); err == nil {
		ign, _ := ignore.CompileIgnoreFile(gitIgnorePath)
		s.GitIgnore = ign
	}

	// 2. Load .codepickerignore if it exists
	cpIgnorePath := filepath.Join(root, ".codepickerignore")
	if _, err := os.Stat(cpIgnorePath); err == nil {
		ign, _ := ignore.CompileIgnoreFile(cpIgnorePath)
		s.CustomIgnore = ign
		// Optional: Let the user know we found it
		// fmt.Println("🚫 Loaded .codepickerignore patterns")
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

		relPath, _ := filepath.Rel(s.Root, path)
		if relPath == "." {
			return nil
		}

		// Check 1: Standard .gitignore
		if s.GitIgnore != nil && s.GitIgnore.MatchesPath(relPath) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Check 2: Custom .codepickerignore (This is the new part)
		if s.CustomIgnore != nil && s.CustomIgnore.MatchesPath(relPath) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Check 3: Writer specific skips (like output files)
		if s.Writer.ShouldSkip(path) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Check 4: Hardcoded Config Directory ignores
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
				return filepath.SkipDir
			}
			if s.Config.IgnoredDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		// Check 5: Extension Filtering
		ext := strings.ToLower(filepath.Ext(path))
		name := strings.ToLower(d.Name())
		if !s.Config.AllowedExts[ext] && !config.IsSpecialFile(name) {
			return nil
		}

		// Visual Feedback
		if s.Writer.Name() != "Tree" {
			fmt.Printf("   Picked: %s\n", relPath)
		}

		return s.Writer.Write(path, relPath)
	})
}

