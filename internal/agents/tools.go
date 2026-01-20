package agents

import (
	"encoding/json"

	"github.com/david22573/codepicker/pkg/openrouter"
)

func getToolsFor(aType AgentType) []openrouter.Tool {

	toolReadFile := openrouter.Tool{
		Type: "function",
		Function: openrouter.ToolFunction{
			Name:        "read_file",
			Description: "Read the contents of a specific file. Optional: specify line range to save tokens.",
			Parameters: json.RawMessage(`{ 
				"type": "object", 
				"properties": { 
					"path": { "type": "string" },
					"start_line": { "type": "integer", "description": "Start line number (1-based, optional)" },
					"end_line": { "type": "integer", "description": "End line number (optional)" }
				}, 
				"required": ["path"] 
			}`),
		},
	}

	toolSearchCode := openrouter.Tool{
		Type: "function",
		Function: openrouter.ToolFunction{
			Name:        "search_code",
			Description: "Search for a keyword or string across all files.",
			Parameters:  json.RawMessage(`{ "type": "object", "properties": { "query": { "type": "string" } }, "required": ["query"] }`),
		},
	}

	toolWriteShadow := openrouter.Tool{
		Type: "function",
		Function: openrouter.ToolFunction{
			Name:        "write_shadow_file",
			Description: "Write code to the shadow workspace.",
			Parameters:  json.RawMessage(`{ "type": "object", "properties": { "path": { "type": "string" }, "content": { "type": "string" } }, "required": ["path", "content"] }`),
		},
	}

	toolRunShell := openrouter.Tool{
		Type: "function",
		Function: openrouter.ToolFunction{
			Name:        "run_shell",
			Description: "Execute a shell command.",
			Parameters:  json.RawMessage(`{ "type": "object", "properties": { "command": { "type": "string" } }, "required": ["command"] }`),
		},
	}

	readTools := []openrouter.Tool{toolReadFile, toolSearchCode}
	writeTools := []openrouter.Tool{toolWriteShadow}
	shellTools := []openrouter.Tool{toolRunShell}

	switch aType {
	case AgentContext:
		return readTools
	case AgentModifier:
		return append(readTools, writeTools...)
	case AgentSystem:
		return append(readTools, shellTools...)
	case AgentQuality:
		return append(readTools, shellTools...)
	case AgentOrchestrator:

		return readTools
	default:
		return readTools
	}
}
