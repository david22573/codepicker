package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/internal/errors"
)

// Sanitize ensures a path is safe, absolute, and contained within the current working directory.
// It resolves symlinks to prevent path traversal attacks.
func Sanitize(path string) (string, error) {
	if path == "" {
		return "", &errors.ValidationError{Field: "path", Message: "path cannot be empty"}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}

	// 1. Resolve absolute path of the root (CWD) to handle its own symlinks
	absRoot, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return "", fmt.Errorf("failed to resolve working directory: %w", err)
	}

	// 2. Get absolute path of the requested target
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	// 3. Resolve symlinks in the target path
	// This ensures /path/to/symlink -> /etc/passwd is detected as /etc/passwd
	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			// If file doesn't exist yet (e.g. output file), check the parent dir
			parent := filepath.Dir(absPath)
			realParent, parentErr := filepath.EvalSymlinks(parent)
			if parentErr != nil {
				return "", fmt.Errorf("invalid parent path: %w", parentErr)
			}
			// Reconstruct the path with the resolved parent
			realPath = filepath.Join(realParent, filepath.Base(absPath))
		} else {
			return "", fmt.Errorf("failed to resolve path symlinks: %w", err)
		}
	}

	// 4. Check containment
	// filepath.Rel returns ".." if the path is outside the base
	rel, err := filepath.Rel(absRoot, realPath)
	if err != nil {
		return "", fmt.Errorf("path validation error: %w", err)
	}

	// If rel starts with ".." or is absolute (on Windows different drives), it's outside
	if strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", &errors.ValidationError{
			Field:   "path",
			Message: "path traversal detected: file is outside working directory",
			Value:   path,
		}
	}

	// 5. Final Blocklist check for sensitive system directories (Double check)
	// Even if inside CWD (unlikely unless CWD is /), we block these.
	forbidden := []string{"/proc", "/sys", "/dev", "/etc", "/var/run"}
	for _, f := range forbidden {
		if strings.HasPrefix(filepath.ToSlash(realPath), f) {
			return "", &errors.ValidationError{
				Field:   "path",
				Message: "access to system directory denied",
				Value:   path,
			}
		}
	}

	return realPath, nil
}

// ValidateOutput performs additional checks specifically for output files
// to prevent overwriting critical project metadata.
func ValidateOutput(out string) error {
	// First, run standard sanitization
	cleanPath, err := Sanitize(out)
	if err != nil {
		return err
	}

	base := filepath.Base(cleanPath)

	// Critical files that should never be overwritten by the tool
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
		return &errors.ValidationError{
			Field:   "outPath",
			Message: "refusing to overwrite critical project file",
			Value:   base,
		}
	}

	// Prevent overwriting hidden files/dirs generally (except specific allowlist if needed)
	if strings.HasPrefix(base, ".") && !strings.HasSuffix(base, ".md") {
		return &errors.ValidationError{
			Field:   "outPath",
			Message: "refusing to overwrite dotfile",
			Value:   base,
		}
	}

	return nil
}
