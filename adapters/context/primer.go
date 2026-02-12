package context

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ProjectPrimer struct {
	Root string
}

func NewProjectPrimer(root string) *ProjectPrimer {
	return &ProjectPrimer{Root: root}
}

// Generate provides a safe string wrapper for Prime(), used by CLI commands.
func (p *ProjectPrimer) Generate() string {
	res, err := p.Prime()
	if err != nil {
		return fmt.Sprintf("Error generating project map: %v", err)
	}
	return res
}

// Prime generates a structured text overview of the repository.
func (p *ProjectPrimer) Prime() (string, error) {
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

		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "vendor" || name == "node_modules" || name == "dist" {
				return filepath.SkipDir
			}
		}

		depth := strings.Count(rel, string(os.PathSeparator))
		indent := strings.Repeat("  ", depth)

		if info.IsDir() {
			sb.WriteString(fmt.Sprintf("%s- %s/\n", indent, info.Name()))
		} else {
			if depth < 4 {
				sb.WriteString(fmt.Sprintf("%s- %s\n", indent, info.Name()))
			}
		}

		return nil
	})

	if err != nil {
		return "", fmt.Errorf("failed to prime project: %w", err)
	}

	goModPath := filepath.Join(p.Root, "go.mod")
	if content, err := os.ReadFile(goModPath); err == nil {
		sb.WriteString("\n### DEPENDENCIES (go.mod)\n")
		sb.WriteString(string(content))
	}

	return sb.String(), nil
}
