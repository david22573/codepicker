package contextgen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/internal/config"
	ignore "github.com/sabhiram/go-gitignore"
)

// GenerateTree returns a visual string representation of the codebase structure
func GenerateTree(root string) (string, error) {
	var sb strings.Builder
	sb.WriteString("### PROJECT STRUCTURE (Shallow Context):\n")
	sb.WriteString("Root: .\n")

	// Load gitignore if available
	var ign *ignore.GitIgnore
	if _, err := os.Stat(filepath.Join(root, ".gitignore")); err == nil {
		ign, _ = ignore.CompileIgnoreFile(filepath.Join(root, ".gitignore"))
	}

	// Use default config to respect ignored dirs like .git, node_modules
	cfg := config.NewConfig()

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		if rel == "." {
			return nil
		}

		// Check .gitignore
		if ign != nil && ign.MatchesPath(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Check standard exclusions
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") || cfg.IgnoredDirs[d.Name()] {
				return filepath.SkipDir
			}
		}

		// Indentation logic
		depth := strings.Count(filepath.ToSlash(rel), "/")
		indent := strings.Repeat("│   ", depth)

		marker := "├──"
		if d.IsDir() {
			marker = "📁"
		}

		sb.WriteString(fmt.Sprintf("%s%s %s\n", indent, marker, d.Name()))
		return nil
	})

	return sb.String(), err
}
