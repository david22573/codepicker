package context

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ProjectPrimer generates a high-level overview of the project structure
// to "prime" the agent before it starts working.
type ProjectPrimer struct {
	ProjectRoot string
}

func NewProjectPrimer(root string) *ProjectPrimer {
	return &ProjectPrimer{ProjectRoot: root}
}

// Generate builds a prompt-ready summary of the project.
func (p *ProjectPrimer) Generate() string {
	var sb strings.Builder
	sb.WriteString("## PROJECT STARTER INFO\n")
	sb.WriteString("You are working in the following project structure:\n\n")

	sb.WriteString("<file_tree>\n")

	// 1. Walk and generate tree
	maxDepth := 4 // Don't go too deep to save tokens
	fileCount := 0

	err := filepath.WalkDir(p.ProjectRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		rel, _ := filepath.Rel(p.ProjectRoot, path)
		if rel == "." {
			return nil
		}

		// Skip hidden/vendor
		if strings.HasPrefix(d.Name(), ".") || d.Name() == "vendor" || d.Name() == "node_modules" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Check depth
		if strings.Count(rel, string(os.PathSeparator)) > maxDepth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Add to tree
		if d.IsDir() {
			sb.WriteString(fmt.Sprintf("- %s/\n", rel))
		} else {
			sb.WriteString(fmt.Sprintf("- %s\n", rel))
			fileCount++
		}

		if fileCount > 200 {
			sb.WriteString("... (truncated)\n")
			return filepath.SkipAll // Stop walking if too huge
		}

		return nil
	})

	if err != nil {
		sb.WriteString("(Error generating tree)\n")
	}
	sb.WriteString("</file_tree>\n\n")

	// 2. Peek at key config files for context
	keyFiles := []string{"go.mod", "package.json", "requirements.txt", "Makefile", "Dockerfile"}
	for _, f := range keyFiles {
		content, err := os.ReadFile(filepath.Join(p.ProjectRoot, f))
		if err == nil {
			// Truncate if too long (e.g., massive package-lock)
			str := string(content)
			if len(str) > 500 {
				str = str[:500] + "\n...(truncated)"
			}
			sb.WriteString(fmt.Sprintf("## Content of %s:\n```\n%s\n```\n\n", f, str))
		}
	}

	return sb.String()
}
