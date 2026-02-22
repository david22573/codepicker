package pathutil

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Clean normalizes a path and prevents directory traversal attacks.
func Clean(relPath string) (string, error) {
	if relPath == "" {
		return "", fmt.Errorf("path cannot be empty")
	}

	clean := filepath.Clean(relPath)

	if filepath.IsAbs(clean) {
		return "", fmt.Errorf("absolute paths are not allowed")
	}

	if strings.Contains(clean, "..") {
		return "", fmt.Errorf("path traversal (..) detected")
	}

	return clean, nil
}