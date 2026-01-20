package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/pkg/openrouter"
	ignore "github.com/sabhiram/go-gitignore"
)

type SearchCodeTool struct {
	Root string // Root directory to search in
}

type searchArgs struct {
	Query string `json:"query"`
}

func (t *SearchCodeTool) Name() string { return "search_code" }

func (t *SearchCodeTool) Description() string {
	return "Search for a keyword or string across all files in the codebase. Returns file paths and matching lines."
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
	var ign *ignore.GitIgnore

	// Attempt to load .gitignore
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

		// Check .gitignore
		if ign != nil && ign.MatchesPath(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Check internal config ignores
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") || (cfg != nil && cfg.IsDirIgnored(d.Name())) {
				return filepath.SkipDir
			}
			return nil
		}

		// Check extensions
		ext := strings.ToLower(filepath.Ext(path))
		isAllowed := cfg == nil || cfg.IsExtensionAllowed(ext)
		// We always allow special files like Dockerfile/Makefile even if extension check fails
		if !isAllowed && !isSpecialFile(d.Name()) {
			return nil
		}

		// Read and Search
		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		lineNum := 1
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, query) {
				if len(line) > 200 {
					line = line[:200] + "..."
				}
				results = append(results, fmt.Sprintf("%s:%d: %s", rel, lineNum, strings.TrimSpace(line)))

				if len(results) > 50 {
					return fmt.Errorf("too_many_results")
				}
			}
			lineNum++
		}
		return nil
	})

	truncated := ""
	if err != nil && err.Error() == "too_many_results" {
		truncated = "\n... (search truncated, be more specific)"
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
