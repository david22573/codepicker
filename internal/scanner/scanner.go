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

	// Initialize Output Strategy (Create file/folder)
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
	// Ensure Writer is closed when scan finishes
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

		// 1. Check if Writer wants to skip this file (e.g. it is the output file)
		if s.Writer.ShouldSkip(path) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		cleanRel := filepath.ToSlash(relPath)

		// 2. Whitelist Check (Git Diff mode)
		if s.Whitelist != nil {
			if !d.IsDir() {
				if !s.Whitelist[cleanRel] {
					return nil
				}
			}
		}

		// 3. Ignore Files Check
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

		// 4. Config Exclusion Check
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
				return filepath.SkipDir
			}
			if s.Config.IgnoredDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		// 5. Extension Check
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
