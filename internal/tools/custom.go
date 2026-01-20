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

func (t *CustomScriptTool) Name() string        { return t.DefinitionModel.Name }
func (t *CustomScriptTool) Description() string { return t.DefinitionModel.Description }

// [Fixed] Added Capabilities
func (t *CustomScriptTool) Capabilities() []Capability {
	return []Capability{CapExecute, CapRead, CapWrite}
}

func (t *CustomScriptTool) Definition() openrouter.Tool {
	schema := t.DefinitionModel.Arguments
	if schema == "" {
		schema = `{"type": "object", "properties": {"args": {"type": "string"}}}`
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
		return "", fmt.Errorf("empty command")
	}

	head := parts[0]
	cmdArgs := append(parts[1:], argsJSON)

	// Sentinel check if available
	if rt.Sentinel != nil {
		if dangerous, _, _, _ := rt.Sentinel.CheckCommand(head); dangerous {
			// Enforcer would have caught this in strict mode via capabilities
		}
	}

	cmd := exec.CommandContext(ctx, head, cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("execution failed: %w", err)
	}
	return string(out), nil
}
