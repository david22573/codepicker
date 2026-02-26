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

	if strings.Contains(relPath, "..") {
		return "", fmt.Errorf("path traversal (..) detected")
	}

	clean := filepath.Clean(relPath)

	if filepath.IsAbs(clean) {
		return "", fmt.Errorf("absolute paths are not allowed")
	}

	return clean, nil
}