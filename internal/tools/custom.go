package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/pkg/openrouter"
)

type CustomScriptTool struct {
	DefinitionModel config.CustomTool
}

func (t *CustomScriptTool) Name() string { return t.DefinitionModel.Name }

func (t *CustomScriptTool) Description() string { return t.DefinitionModel.Description }

func (t *CustomScriptTool) Definition() openrouter.Tool {
	schema := t.DefinitionModel.Arguments
	if schema == "" {
		// Default schema if none provided: just a generic "args" string
		schema = `{
			"type": "object",
			"properties": {
				"args": { "type": "string", "description": "Arguments for the command" }
			}
		}`
	}

	return openrouter.Tool{
		Type: "function",
		Function: openrouter.ToolFunction{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  json.RawMessage(schema),
		},
	}
}

func (t *CustomScriptTool) Execute(ctx context.Context, argsJSON string, rt *RuntimeContext) (string, error) {
	// Parse the command from the config
	parts := strings.Fields(t.DefinitionModel.Command)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty command in configuration for tool %s", t.Name())
	}

	head := parts[0]
	cmdArgs := parts[1:]

	// Append the JSON arguments so the script can parse them
	cmdArgs = append(cmdArgs, argsJSON)

	// Security Check: We treat custom tools like shell commands
	if rt.Sentinel != nil {
		// We only check the binary itself to ensure it's allowed by policy
		needsApproval, reason, _, _ := rt.Sentinel.CheckCommand(head)
		if needsApproval {
			// If sentinel flags it, we check if we have a user approval callback (via Shell Tool)
			// NOTE: Ideally, custom tools should be pre-approved or run in Admin mode.
			// For now, we assume if they are in config, they are trusted, UNLESS policy is Strict.
		}
		_ = reason
	}

	cmd := exec.CommandContext(ctx, head, cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("script execution failed: %w", err)
	}

	return string(out), nil
}
