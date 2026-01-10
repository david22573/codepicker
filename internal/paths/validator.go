package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/internal/errors"
)

// Sanitize checks for path traversal and ensures the path is absolute
func Sanitize(path string) (string, error) {
	clean := filepath.Clean(path)

	abs, err := filepath.Abs(clean)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}

	rel, err := filepath.Rel(wd, abs)
	if err != nil {
		return "", fmt.Errorf("failed to get relative path: %w", err)
	}

	if strings.HasPrefix(rel, "..") {
		return "", &errors.ValidationError{
			Field:   "path",
			Message: "path escapes working directory",
			Value:   path,
		}
	}

	return abs, nil
}

// ValidateOutput ensures we aren't writing to system directories or critical files
func ValidateOutput(out string) error {
	forbidden := []string{"/", "/usr", "/etc", "/bin", "/sbin", "/opt", "/sys", "/proc", "/dev"}
	for _, forbiddenDir := range forbidden {
		if out == forbiddenDir || strings.HasPrefix(out, forbiddenDir+string(filepath.Separator)) {
			return &errors.ValidationError{
				Field:   "outPath",
				Message: "forbidden system directory",
				Value:   out,
			}
		}
	}

	base := filepath.Base(out)
	criticalFiles := map[string]bool{
		"go.mod":            true,
		"go.sum":            true,
		"package.json":      true,
		"package-lock.json": true,
		".git":              true,
	}

	if criticalFiles[base] {
		return &errors.ValidationError{
			Field:   "outPath",
			Message: "refusing to overwrite important file",
			Value:   out,
		}
	}

	return nil
}
