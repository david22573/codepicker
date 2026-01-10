package scanner

import (
	"context"
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
	CustomIgnore *ignore.GitIgnore
}

func NewScanner(root string, w writer.OutputStrategy, cfg *config.Config) *Scanner {
	s := &Scanner{
		Root:   root,
		Writer: w,
		Config: cfg,
	}

	gitIgnorePath := filepath.Join(root, ".gitignore")
	if _, err := os.Stat(gitIgnorePath); err == nil {
		ign, _ := ignore.CompileIgnoreFile(gitIgnorePath)
		s.GitIgnore = ign
	}

	cpIgnorePath := filepath.Join(root, ".codepickerignore")
	if _, err := os.Stat(cpIgnorePath); err == nil {
		ign, _ := ignore.CompileIgnoreFile(cpIgnorePath)
		s.CustomIgnore = ign
	}

	return s
}

// Scan now accepts a context for cancellation
func (s *Scanner) Scan(ctx context.Context) error {
	if err := s.Writer.Init(); err != nil {
		return fmt.Errorf("writer init failed: %w", err)
	}
	defer s.Writer.Close()

	return filepath.WalkDir(s.Root, func(path string, d os.DirEntry, err error) error {
		// 1. Check for cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err != nil {
			return err
		}

		relPath, _ := filepath.Rel(s.Root, path)
		if relPath == "." {
			return nil
		}

		if s.GitIgnore != nil && s.GitIgnore.MatchesPath(relPath) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if s.CustomIgnore != nil && s.CustomIgnore.MatchesPath(relPath) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if s.Writer.ShouldSkip(path) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
				return filepath.SkipDir
			}
			if s.Config.IgnoredDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		name := strings.ToLower(d.Name())
		if !s.Config.AllowedExts[ext] && !config.IsSpecialFile(name) {
			return nil
		}

		if s.Writer.Name() != "Tree" {
			fmt.Printf("   Picked: %s\n", relPath)
		}

		return s.Writer.Write(path, relPath)
	})
}

