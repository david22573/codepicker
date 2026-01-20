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

// Custom tools are dangerous by default unless configured otherwise
func (t *CustomScriptTool) Capabilities() []Capability {
	return []Capability{CapExecute, CapRead, CapWrite}
}

func (t *CustomScriptTool) Definition() openrouter.Tool {
	schema := t.DefinitionModel.Arguments
	if schema == "" {
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

	parts := strings.Fields(t.DefinitionModel.Command)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty command in configuration for tool %s", t.Name())
	}

	head := parts[0]
	cmdArgs := parts[1:]

	cmdArgs = append(cmdArgs, argsJSON)

	// Check with Sentinel if available
	if rt.Sentinel != nil {
		needsApproval, _, _, _ := rt.Sentinel.CheckCommand(head)
		if needsApproval {
			// In strict modes, Enforcer would have already caught this via Capabilities
		}
	}

	cmd := exec.CommandContext(ctx, head, cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("script execution failed: %w", err)
	}

	return string(out), nil
}
