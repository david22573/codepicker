package contextgen

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/paths"
	"github.com/david22573/codepicker/internal/scanner"
	"github.com/david22573/codepicker/internal/writer"
)

type Options struct {
	SrcDir      string
	FocusFiles  []string
	Minify      bool
	IncludeExts string
	IgnoreDirs  string
}

// Generate scans the codebase based on options and returns the full context string.
func Generate(ctx context.Context, opts Options, log logger.Logger) (string, error) {
	// 1. Create Temp File
	tmpFile, err := os.CreateTemp("", "codepicker_context_*.md")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath) // Auto-cleanup

	// 2. Initialize Writer
	w := writer.NewConcatStrategy(tmpPath, opts.Minify, false)
	if err := w.Init(); err != nil {
		return "", fmt.Errorf("failed to init writer: %w", err)
	}

	// 3. Scan or Focus
	if len(opts.FocusFiles) > 0 {
		log.Info(fmt.Sprintf("Focus mode: %d file(s) selected", len(opts.FocusFiles)))
		for _, f := range opts.FocusFiles {
			abs, _ := filepath.Abs(f)
			rel, _ := filepath.Rel(".", abs)
			if err := w.Write(abs, rel); err != nil {
				log.Warn(fmt.Sprintf("Failed to write %s: %v", rel, err))
			}
		}
	} else {
		// Full Scan
		absSrc, err := paths.Sanitize(opts.SrcDir)
		if err != nil {
			return "", err
		}

		cfg := config.NewConfig()
		if opts.IncludeExts != "" {
			cfg.AddAllowedExtensions(strings.Split(opts.IncludeExts, ","))
		}
		if opts.IgnoreDirs != "" {
			cfg.AddIgnoredDirs(strings.Split(opts.IgnoreDirs, ","))
		}

		s := scanner.NewScanner(absSrc, w, cfg, log)
		if err := s.Scan(ctx); err != nil {
			return "", err
		}
	}

	if err := w.Close(); err != nil {
		return "", err
	}

	// 4. Read Result
	content, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", err
	}

	return string(content), nil
}
