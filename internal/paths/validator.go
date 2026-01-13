package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/internal/errors"
)

func Sanitize(path string) (string, error) {
	if path == "" {
		return "", errors.NewValidationError(
			"path",
			"path cannot be empty",
			path,
		)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}

	absRoot, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return "", fmt.Errorf("failed to resolve working directory: %w", err)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			parent := filepath.Dir(absPath)
			realParent, parentErr := filepath.EvalSymlinks(parent)
			if parentErr != nil {
				return "", fmt.Errorf("invalid parent path: %w", parentErr)
			}
			realPath = filepath.Join(realParent, filepath.Base(absPath))
		} else {
			return "", fmt.Errorf("failed to resolve path symlinks: %w", err)
		}
	}

	rel, err := filepath.Rel(absRoot, realPath)
	if err != nil {
		return "", fmt.Errorf("path validation error: %w", err)
	}

	if strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", errors.NewValidationError(
			"path",
			"path traversal detected: file is outside working directory",
			path,
		)
	}

	forbidden := []string{"/proc", "/sys", "/dev", "/etc", "/var/run"}
	for _, f := range forbidden {
		if strings.HasPrefix(filepath.ToSlash(realPath), f) {
			return "", errors.NewValidationError(
				"path",
				"access to system directory denied",
				path,
			)
		}
	}

	return realPath, nil
}

func ValidateOutput(out string) error {
	cleanPath, err := Sanitize(out)
	if err != nil {
		return err
	}

	base := filepath.Base(cleanPath)

	criticalFiles := map[string]bool{
		"go.mod":            true,
		"go.sum":            true,
		"package.json":      true,
		"package-lock.json": true,
		".git":              true,
		".env":              true,
		".gitignore":        true,
		"Makefile":          true,
	}

	if criticalFiles[base] {
		return errors.NewValidationError(
			"outPath",
			"refusing to overwrite critical project file",
			base,
		)
	}

	if strings.HasPrefix(base, ".") && !strings.HasSuffix(base, ".md") {
		return errors.NewValidationError(
			"outPath",
			"refusing to overwrite dotfile",
			base,
		)
	}

	return nil
}
