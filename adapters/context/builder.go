package context

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/david22573/codepicker/domain/errors"
)

// Config defines the rules for context generation
type Config struct {
	MaxTokens       int
	IncludePatterns []string
	ExcludePatterns []string
	ProjectRoot     string
}

type Builder struct{}

func NewBuilder() *Builder {
	return &Builder{}
}

// Build generates a markdown string containing the project context
func (b *Builder) Build(cfg Config) (string, error) {
	var sb strings.Builder

	// 1. Discovery: Collect all candidate files
	candidates, err := b.scanProject(cfg)
	if err != nil {
		return "", errors.NewSystem("context.Build", "scan failed", err)
	}

	// 2. The Map: Generate and append the file tree first
	// This gives the LLM a high-level mental model before reading code
	sb.WriteString("# Project Context\n\n")
	sb.WriteString("## File Tree\n")
	sb.WriteString("```text\n")
	sb.WriteString(b.generateTree(candidates))
	sb.WriteString("```\n\n")

	// 3. Smart Sort: Prioritize Core Logic over Implementation Details
	// (e.g. main.go and domain/ interfaces come before infrastructure)
	sort.SliceStable(candidates, func(i, j int) bool {
		return scoreFile(candidates[i]) > scoreFile(candidates[j])
	})

	// 4. Content Dump: Read files and enforce budget
	sb.WriteString("## Source Code\n\n")

	currentTokens := 0
	filesProcessed := 0

	for _, relPath := range candidates {
		fullPath := filepath.Join(cfg.ProjectRoot, relPath)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue // Skip unreadable
		}

		// Improved Heuristic: Code usually averages 3.5 chars/token
		estTokens := len(content) / 3

		// Add header tokens overhead
		estTokens += 20

		if currentTokens+estTokens > cfg.MaxTokens {
			sb.WriteString(fmt.Sprintf("\n> ⚠️ Context limit reached (%d/%d tokens). Skipping remaining %d files.\n",
				currentTokens, cfg.MaxTokens, len(candidates)-filesProcessed))
			break
		}

		// Determine language for syntax highlighting
		lang := detectLanguage(relPath)

		// Use XML-style wrapping for clearer boundary detection by Agents
		sb.WriteString(fmt.Sprintf("<file path=\"%s\">\n", relPath))
		sb.WriteString(fmt.Sprintf("```%s\n", lang))
		sb.WriteString(string(content))
		sb.WriteString("\n```\n")
		sb.WriteString("</file>\n\n")

		currentTokens += estTokens
		filesProcessed++
	}

	return sb.String(), nil
}

// scanProject walks the directory and applies include/exclude rules
func (b *Builder) scanProject(cfg Config) ([]string, error) {
	var candidates []string

	err := filepath.Walk(cfg.ProjectRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			// Always exclude hidden dirs and vendor/node_modules
			if strings.HasPrefix(info.Name(), ".") && info.Name() != "." {
				return filepath.SkipDir
			}
			if info.Name() == "vendor" || info.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, _ := filepath.Rel(cfg.ProjectRoot, path)
		filename := filepath.Base(path)

		// Check User Excludes
		for _, pat := range cfg.ExcludePatterns {
			// 1. Check strict path match (e.g. "cmd/internal")
			matchedPath, _ := filepath.Match(pat, relPath)

			// 2. Check filename match (e.g. "*.log" or "go.sum")
			matchedName, _ := filepath.Match(pat, filename)

			if matchedPath || matchedName {
				return nil // Skip this file
			}
		}

		// Check User Includes (if specified)
		if len(cfg.IncludePatterns) > 0 {
			included := false
			for _, pat := range cfg.IncludePatterns {
				matched, _ := filepath.Match(pat, relPath)
				if matched {
					included = true
					break
				}
			}
			if !included {
				return nil
			}
		}

		candidates = append(candidates, relPath)
		return nil
	})

	return candidates, err
}

// generateTree creates a visual representation of the selected files
func (b *Builder) generateTree(files []string) string {
	// Re-sort alphabetically for the tree view only
	treeFiles := make([]string, len(files))
	copy(treeFiles, files)
	sort.Strings(treeFiles)

	var sb strings.Builder
	for _, f := range treeFiles {
		sb.WriteString(fmt.Sprintf("├── %s\n", f))
	}
	return sb.String()
}

// scoreFile assigns a priority score. Higher is better.
func scoreFile(path string) int {
	base := strings.ToLower(filepath.Base(path))
	dir := strings.ToLower(filepath.Dir(path))

	score := 10 // Default score

	// CRITICAL: Entry points and configuration
	if base == "main.go" || base == "go.mod" || base == "makefile" {
		return 100
	}

	// HIGH: Domain logic (Interfaces & Entities)
	if strings.Contains(dir, "domain") {
		return 80
	}

	// MEDIUM-HIGH: Command definitions
	if strings.Contains(dir, "cmd") {
		return 70
	}

	// MEDIUM: Core Adapters (Business logic implementation)
	if strings.Contains(dir, "adapters") {
		return 60
	}

	// LOW: Infrastructure (Details)
	if strings.Contains(dir, "infra") {
		return 40
	}

	// LOWEST: Tests
	if strings.HasSuffix(base, "_test.go") {
		return 0
	}

	return score
}

// detectLanguage maps file extensions to markdown language identifiers
func detectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	base := strings.ToLower(filepath.Base(path))

	switch {
	case base == "makefile":
		return "makefile"
	case base == "dockerfile":
		return "dockerfile"
	case ext == ".go":
		return "go"
	case ext == ".js":
		return "javascript"
	case ext == ".ts":
		return "typescript"
	case ext == ".json":
		return "json"
	case ext == ".md":
		return "markdown"
	case ext == ".yml", ext == ".yaml":
		return "yaml"
	case ext == ".html":
		return "html"
	case ext == ".css":
		return "css"
	case ext == ".sh":
		return "bash"
	case ext == ".sql":
		return "sql"
	default:
		return "text"
	}
}
