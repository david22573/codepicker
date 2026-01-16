package agent

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/pkg/openrouter"
)

// Base built-in tools
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
			Description: "Execute a shell command. Use this for 'ls', 'grep', 'go test', etc.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"command": { "type": "string", "description": "The full shell command string" }
				},
				"required": ["command"]
			}`),
		},
	},
}

// GetTools merges built-in tools with custom tools defined in config
func GetTools(cfg *config.ConfigFile) []openrouter.Tool {
	tools := make([]openrouter.Tool, len(builtInTools))
	copy(tools, builtInTools)

	if cfg == nil {
		return tools
	}

	for _, ct := range cfg.CustomTools {
		// Default to generic string input if no schema provided
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

// ExecuteCustomTool handles the mapping from AI function call to actual shell command
func ExecuteCustomTool(name string, argsJSON string, cfg *config.ConfigFile) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("no config loaded")
	}

	for _, ct := range cfg.CustomTools {
		if ct.Name == name {
			// Parse arguments to safely inject them (simple version injects raw JSON or formatted string)
			// For safety/simplicity in this phase, we append the raw JSON args to the command
			// In production, you'd want robust template injection here.

			parts := strings.Fields(ct.Command)
			if len(parts) == 0 {
				return "", fmt.Errorf("empty command")
			}

			head := parts[0]
			cmdArgs := parts[1:]
			cmdArgs = append(cmdArgs, argsJSON) // Pass JSON args to the script

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
