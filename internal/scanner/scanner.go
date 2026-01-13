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
	Whitelist    map[string]bool
}

func NewScanner(root string, w writer.OutputStrategy, cfg *config.Config, log logger.Logger) *Scanner {
	s := &Scanner{
		Root:   root,
		Writer: w,
		Config: cfg,
		Logger: log,
	}

	// Move writer init here to ensure it's ready before scan starts
	if err := s.Writer.Init(); err != nil {
		s.Logger.Error(fmt.Sprintf("Failed to init writer: %v", err))
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

func (s *Scanner) SetWhitelist(files map[string]bool) {
	s.Whitelist = files
}

func (s *Scanner) Scan(ctx context.Context) error {
	// Use WalkDir (more efficient than Walk)
	return filepath.WalkDir(s.Root, func(path string, d os.DirEntry, err error) error {
		// 1. Check Cancellation Signal
		select {
		case <-ctx.Done():
			// Return special error to stop WalkDir immediately
			return filepath.SkipAll
		default:
		}

		if err != nil {
			if path == s.Root {
				return err
			}
			// Don't crash on permission errors, just warn and skip
			s.Logger.Warn(fmt.Sprintf("Skipping %s: access denied or error: %v", path, err))
			return filepath.SkipDir
		}

		// Calculate relative path for filtering
		relPath, relErr := filepath.Rel(s.Root, path)
		if relErr != nil {
			return nil
		}
		if relPath == "." {
			return nil
		}

		// 2. Check if Writer says skip (e.g., output file)
		if s.Writer.ShouldSkip(path) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		cleanRel := filepath.ToSlash(relPath)

		// 3. Diff Mode Whitelist
		if s.Whitelist != nil {
			if !d.IsDir() {
				if !s.Whitelist[cleanRel] {
					return nil
				}
			}
		}

		// 4. GitIgnore checks
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

		// 5. Directory filtering
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
				return filepath.SkipDir
			}
			if s.Config.IgnoredDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		// 6. Extension filtering
		ext := strings.ToLower(filepath.Ext(path))
		name := strings.ToLower(d.Name())

		// Logic: If it's not an allowed extension AND not a special file (like Dockerfile), skip it
		if !s.Config.AllowedExts[ext] && !config.IsSpecialFile(name) {
			return nil
		}

		// Logging
		if s.Writer.Name() != "Tree" {
			s.Logger.Info(fmt.Sprintf("Picked: %s", relPath))
		}

		// 7. Write to output
		// This now delegates opening the file to the Writer, which handles closing it.
		writeErr := s.Writer.Write(path, relPath)
		if writeErr != nil {
			s.Logger.Warn(fmt.Sprintf("Failed to write %s: %v", relPath, writeErr))
			// We don't abort the whole scan on a single file error, usually.
			return nil
		}

		return nil
	})
}
