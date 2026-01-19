package agents

import (
	"encoding/json"

	"github.com/david22573/codepicker/pkg/openrouter"
)

// getToolsFor assigns specific permissions (tools) to specific agents
func getToolsFor(aType AgentType) []openrouter.Tool {

	// Define tools locally to avoid dependency on legacy package internals
	toolReadFile := openrouter.Tool{
		Type: "function",
		Function: openrouter.ToolFunction{
			Name:        "read_file",
			Description: "Read the contents of a specific file from the project.",
			Parameters:  json.RawMessage(`{ "type": "object", "properties": { "path": { "type": "string" } }, "required": ["path"] }`),
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

	// Toolset assignments
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
		// Orchestrator tools will be added in Phase 2
		return readTools
	default:
		return readTools
	}
}
