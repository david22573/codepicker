package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/pkg/openrouter"
	ignore "github.com/sabhiram/go-gitignore"
)

var builtInTools = []openrouter.Tool{
	{
		Type: "function",
		Function: openrouter.ToolFunction{
			Name:        "read_file",
			Description: "Read the contents of a specific file from the project.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": { "type": "string", "description": "Relative path to the file (e.g., 'cmd/main.go')" }
				},
				"required": ["path"]
			}`),
		},
	},
	{
		Type: "function",
		Function: openrouter.ToolFunction{
			Name:        "search_code",
			Description: "Search for a keyword or string across all files in the codebase. Returns file paths and matching lines.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": { "type": "string", "description": "The string to search for" }
				},
				"required": ["query"]
			}`),
		},
	},
	{
		Type: "function",
		Function: openrouter.ToolFunction{
			Name:        "write_shadow_file",
			Description: "Write code to the shadow workspace. Use this to propose changes or create new files.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": { "type": "string", "description": "Relative path to the file" },
					"content": { "type": "string", "description": "The full content of the file" }
				},
				"required": ["path", "content"]
			}`),
		},
	},
	{
		Type: "function",
		Function: openrouter.ToolFunction{
			Name:        "run_shell",
			Description: "Execute a shell command. Use this for 'ls', 'go test', etc. prefer search_code for finding code.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"command": { "type": "string", "description": "The full shell command string" }
				},
				"required": ["command"]
			}`),
		},
	},
	{
		Type: "function",
		Function: openrouter.ToolFunction{
			Name:        "delegate_task",
			Description: "Delegate a sub-task to a worker agent. Use this for implementation, reading large files, or executing repetitive tasks.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"instruction": { "type": "string", "description": "Specific instructions for the worker" },
					"context_files": { "type": "string", "description": "Comma-separated list of files the worker needs to read" }
				},
				"required": ["instruction"]
			}`),
		},
	},
}

func GetTools(cfg *config.ConfigFile) []openrouter.Tool {
	tools := make([]openrouter.Tool, len(builtInTools))
	copy(tools, builtInTools)

	if cfg == nil {
		return tools
	}

	for _, ct := range cfg.CustomTools {
		params := ct.Arguments
		if params == "" {
			params = `{
				"type": "object",
				"properties": {
					"args": { "type": "string", "description": "Arguments for the command" }
				}
			}`
		}

		tools = append(tools, openrouter.Tool{
			Type: "function",
			Function: openrouter.ToolFunction{
				Name:        ct.Name,
				Description: ct.Description,
				Parameters:  json.RawMessage(params),
			},
		})
	}

	return tools
}

func ExecuteCustomTool(name string, argsJSON string, cfg *config.ConfigFile) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("no config loaded")
	}

	for _, ct := range cfg.CustomTools {
		if ct.Name == name {
			parts := strings.Fields(ct.Command)
			if len(parts) == 0 {
				return "", fmt.Errorf("empty command")
			}

			head := parts[0]
			cmdArgs := parts[1:]
			cmdArgs = append(cmdArgs, argsJSON)

			cmd := exec.Command(head, cmdArgs...)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return string(out), fmt.Errorf("execution failed: %w", err)
			}
			return string(out), nil
		}
	}
	return "", fmt.Errorf("tool not found: %s", name)
}

func PerformSearch(root, query string) (string, error) {
	var results []string
	cfg := config.NewConfig()

	var ign *ignore.GitIgnore
	if _, err := os.Stat(filepath.Join(root, ".gitignore")); err == nil {
		ign, _ = ignore.CompileIgnoreFile(filepath.Join(root, ".gitignore"))
	}

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		if rel == "." {
			return nil
		}

		if ign != nil && ign.MatchesPath(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") || cfg.IgnoredDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !cfg.AllowedExts[ext] && !config.IsSpecialFile(strings.ToLower(d.Name())) {
			return nil
		}

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
				results = append(results, fmt.Sprintf("%s:%d: %s", rel, lineNum, strings.TrimSpace(line)))
				foundInFile = true

				if len(results) > 50 {
					return fmt.Errorf("too many results")
				}
			}
			lineNum++
		}

		if err := scanner.Err(); err != nil {
			return nil
		}

		_ = foundInFile
		return nil
	})

	if len(results) == 0 {
		return "No matches found.", nil
	}

	finalOutput := strings.Join(results, "\n")
	if err != nil && err.Error() == "too many results" {
		finalOutput += "\n... (search truncated, be more specific)"
	}

	return finalOutput, nil
}
