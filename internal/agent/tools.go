package agent

import (
	"encoding/json"

	"github.com/david22573/codepicker/pkg/openrouter"
)

var Tools = []openrouter.Tool{
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
