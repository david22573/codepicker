package context

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ProjectPrimer generates a map of the file structure to help the agent navigate.
type ProjectPrimer struct {
	Root string
}

func NewProjectPrimer(root string) *ProjectPrimer {
	return &ProjectPrimer{Root: root}
}

// Generate provides a default deep map (depth 4), compatible with existing callers.
func (p *ProjectPrimer) Generate() string {
	return p.GenerateWithDepth(4)
}

// GenerateShallow provides a high-level overview (depth 2), perfect for initial planning.
func (p *ProjectPrimer) GenerateShallow() string {
	return p.GenerateWithDepth(2)
}

// GenerateWithDepth generates a structured text overview with a specific recursion limit.
func (p *ProjectPrimer) GenerateWithDepth(maxDepth int) string {
	res, err := p.prime(maxDepth)
	if err != nil {
		return fmt.Sprintf("Error generating project map: %v", err)
	}
	return res
}

func (p *ProjectPrimer) prime(maxDepth int) (string, error) {
	var sb strings.Builder
	sb.WriteString("### PROJECT MAP & STRUCTURE\n")

	err := filepath.Walk(p.Root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, _ := filepath.Rel(p.Root, path)
		if rel == "." {
			return nil
		}

		// Calculate depth
		depth := strings.Count(rel, string(os.PathSeparator)) + 1

		// Skip if deeper than requested
		if depth > maxDepth {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Indentation for tree structure
		indent := strings.Repeat("  ", depth-1)

		if info.IsDir() {
			name := info.Name()
			// Smart filtering: Skip common noise directories
			if strings.HasPrefix(name, ".") || name == "vendor" || name == "node_modules" || name == "dist" || name == "tmp" {
				return filepath.SkipDir
			}
			sb.WriteString(fmt.Sprintf("%s- %s/\n", indent, info.Name()))
		} else {
			sb.WriteString(fmt.Sprintf("%s- %s\n", indent, info.Name()))
		}

		return nil
	})

	if err != nil {
		return "", fmt.Errorf("failed to prime project: %w", err)
	}

	// Always include go.mod context if it exists, as it's critical for understanding dependencies
	goModPath := filepath.Join(p.Root, "go.mod")
	if content, err := os.ReadFile(goModPath); err == nil {
		sb.WriteString("\n### DEPENDENCIES (go.mod)\n")
		// Truncate go.mod if it's huge, just keeping the requires
		sb.WriteString(string(content))
	}

	return sb.String(), nil
}
