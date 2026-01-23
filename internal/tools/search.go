package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/internal/shadow"
	"github.com/david22573/codepicker/pkg/openrouter"
	ignore "github.com/sabhiram/go-gitignore"
)

type SearchCodeTool struct {
	Root   string
	Shadow *shadow.Manager
}

type searchArgs struct {
	Query string `json:"query"`
}

func (t *SearchCodeTool) Name() string { return "search_code" }

func (t *SearchCodeTool) Description() string {
	return "Search for a keyword or string across all files in the codebase (includes pending changes)."
}

func (t *SearchCodeTool) Capabilities() []Capability {
	return []Capability{CapRead}
}

func (t *SearchCodeTool) Definition() openrouter.Tool {
	return openrouter.Tool{
		Type: "function",
		Function: openrouter.ToolFunction{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": { "type": "string", "description": "The string to search for" }
				},
				"required": ["query"]
			}`),
		},
	}
}

func (t *SearchCodeTool) Execute(ctx context.Context, argsJSON string, rt *RuntimeContext) (string, error) {
	var args searchArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	return t.performSearch(args.Query, rt.Config)
}

func (t *SearchCodeTool) performSearch(query string, cfg ConfigProvider) (string, error) {
	var results []string
	shadowedPaths := make(map[string]bool)

	// Shared logic for processing a file match
	processFile := func(path, rel string) error {
		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		lineNum := 1
		foundInFile := false

		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, query) {
				if len(line) > 200 {
					line = line[:200] + "..."
				}

				// Tag shadow results so the agent knows this is a pending change
				prefix := ""
				if t.Shadow != nil && strings.HasPrefix(path, t.Shadow.ShadowRoot) {
					prefix = "[PENDING] "
				}

				results = append(results, fmt.Sprintf("%s%s:%d: %s", prefix, rel, lineNum, strings.TrimSpace(line)))
				foundInFile = true

				if len(results) > 50 {
					return fmt.Errorf("too_many_results")
				}
			}
			lineNum++
		}

		_ = foundInFile
		return nil
	}

	// 1. Search Shadow Directory First (if available)
	if t.Shadow != nil {
		_ = filepath.WalkDir(t.Shadow.ShadowRoot, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if d.Name() == "manifest.json" {
				return nil
			}

			rel, _ := filepath.Rel(t.Shadow.ShadowRoot, path)
			shadowedPaths[rel] = true // Mark this path as shadowed

			// Apply standard extension filtering even to shadow files
			ext := strings.ToLower(filepath.Ext(path))
			isAllowed := cfg == nil || cfg.IsExtensionAllowed(ext)
			if !isAllowed && !isSpecialFile(d.Name()) {
				return nil
			}

			return processFile(path, rel)
		})
	}

	if len(results) > 50 {
		return strings.Join(results, "\n") + "\n... (search truncated)", nil
	}

	// 2. Search Source Directory (skipping shadowed files)
	var ign *ignore.GitIgnore
	if _, err := os.Stat(filepath.Join(t.Root, ".gitignore")); err == nil {
		ign, _ = ignore.CompileIgnoreFile(filepath.Join(t.Root, ".gitignore"))
	}

	err := filepath.WalkDir(t.Root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(t.Root, path)
		if rel == "." {
			return nil
		}

		// Skip if this file was already searched in the shadow pass
		if shadowedPaths[rel] {
			if d.IsDir() {
				return filepath.SkipDir // Don't traverse shadowed directories if they exist
			}
			return nil
		}

		if ign != nil && ign.MatchesPath(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") || (cfg != nil && cfg.IsDirIgnored(d.Name())) {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		isAllowed := cfg == nil || cfg.IsExtensionAllowed(ext)
		if !isAllowed && !isSpecialFile(d.Name()) {
			return nil
		}

		return processFile(path, rel)
	})

	truncated := ""
	if err != nil && err.Error() == "too_many_results" {
		truncated = "\n... (search truncated)"
	}
	if len(results) == 0 {
		return "No matches found.", nil
	}
	return strings.Join(results, "\n") + truncated, nil
}

func isSpecialFile(name string) bool {
	special := map[string]bool{
		"makefile": true, "dockerfile": true, "gemfile": true,
		"jenkinsfile": true, "procfile": true, "readme": true,
		"go.mod": true, "package.json": true,
	}
	return special[strings.ToLower(name)]
}
