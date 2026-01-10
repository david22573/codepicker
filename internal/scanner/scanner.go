package scanner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/writer"
	ignore "github.com/sabhiram/go-gitignore"
)

type Scanner struct {
	Root         string
	Writer       writer.OutputStrategy
	Config       *config.Config
	GitIgnore    *ignore.GitIgnore
	CustomIgnore *ignore.GitIgnore
	Logger       logger.Logger
	// Whitelist acts as a strict filter. If not nil, only files in this map are processed.
	Whitelist map[string]bool
}

func NewScanner(root string, w writer.OutputStrategy, cfg *config.Config, log logger.Logger) *Scanner {
	s := &Scanner{
		Root:   root,
		Writer: w,
		Config: cfg,
		Logger: log,
	}

	gitIgnorePath := filepath.Join(root, ".gitignore")
	if _, err := os.Stat(gitIgnorePath); err == nil {
		ign, _ := ignore.CompileIgnoreFile(gitIgnorePath)
		s.GitIgnore = ign
		s.Logger.Debug("Loaded .gitignore")
	}

	cpIgnorePath := filepath.Join(root, ".codepickerignore")
	if _, err := os.Stat(cpIgnorePath); err == nil {
		ign, _ := ignore.CompileIgnoreFile(cpIgnorePath)
		s.CustomIgnore = ign
		s.Logger.Debug("Loaded .codepickerignore")
	}

	return s
}

// SetWhitelist restricts the scanner to only process the given relative paths.
func (s *Scanner) SetWhitelist(files map[string]bool) {
	s.Whitelist = files
}

func (s *Scanner) Scan(ctx context.Context) error {
	if err := s.Writer.Init(); err != nil {
		return fmt.Errorf("writer init failed: %w", err)
	}
	defer s.Writer.Close()

	return filepath.WalkDir(s.Root, func(path string, d os.DirEntry, err error) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err != nil {
			if path == s.Root {
				return err
			}
			s.Logger.Warn(fmt.Sprintf("Skipping %s: access denied", path))
			return nil
		}

		relPath, _ := filepath.Rel(s.Root, path)
		if relPath == "." {
			return nil
		}

		// Normalize path separators for consistent map lookup
		cleanRel := filepath.ToSlash(relPath)

		// 1. Strict Whitelist Check (for Git Diff mode)
		if s.Whitelist != nil {
			if d.IsDir() {
				// We don't skip dirs in whitelist mode, we just traverse them
				// to find the specific whitelisted files inside.
			} else {
				if !s.Whitelist[cleanRel] {
					return nil // Skip this file as it's not in the whitelist
				}
			}
		}

		// 2. Ignore Checks
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
			s.Logger.Info(fmt.Sprintf("Picked: %s", relPath))
		}

		return s.Writer.Write(path, relPath)
	})
}
