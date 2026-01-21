package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/pkg/openrouter"
	ignore "github.com/sabhiram/go-gitignore"
)

type ListFilesTool struct {
	Root string
}

func (t *ListFilesTool) Name() string { return "list_files" }

func (t *ListFilesTool) Description() string {
	return "List all files in the project to understand the directory structure and locate files."
}

func (t *ListFilesTool) Capabilities() []Capability {
	return []Capability{CapRead}
}

func (t *ListFilesTool) Definition() openrouter.Tool {
	return openrouter.Tool{
		Type: "function",
		Function: openrouter.ToolFunction{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  json.RawMessage(`{"type": "object", "properties": {}}`),
		},
	}
}

func (t *ListFilesTool) Execute(ctx context.Context, argsJSON string, rt *RuntimeContext) (string, error) {
	var ign *ignore.GitIgnore
	if _, err := os.Stat(filepath.Join(t.Root, ".gitignore")); err == nil {
		ign, _ = ignore.CompileIgnoreFile(filepath.Join(t.Root, ".gitignore"))
	}

	var sb strings.Builder
	sb.WriteString("### PROJECT FILES:\n")

	err := filepath.WalkDir(t.Root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		rel, _ := filepath.Rel(t.Root, path)
		if rel == "." {
			return nil
		}

		// Ignore .git and other hidden/ignored directories
		if ign != nil && ign.MatchesPath(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") || (rt.Config != nil && rt.Config.IsDirIgnored(d.Name())) {
				return filepath.SkipDir
			}
			return nil
		}

		// Check extensions
		ext := strings.ToLower(filepath.Ext(path))
		isAllowed := rt.Config == nil || rt.Config.IsExtensionAllowed(ext)
		if !isAllowed && !config.IsSpecialFile(d.Name()) {
			return nil
		}

		sb.WriteString(fmt.Sprintf("- %s\n", rel))
		return nil
	})

	if err != nil {
		return "", fmt.Errorf("failed to list files: %w", err)
	}

	return sb.String(), nil
}
