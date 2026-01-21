package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/david22573/codepicker/internal/mcp"
	"github.com/david22573/codepicker/pkg/openrouter"
	mcp_protocol "github.com/mark3labs/mcp-go/mcp"
)

type MCPToolAdapter struct {
	ServerName string
	Def        mcp_protocol.Tool
	Manager    *mcp.Manager
}

func NewMCPToolAdapter(srvName string, def mcp_protocol.Tool, mgr *mcp.Manager) *MCPToolAdapter {
	return &MCPToolAdapter{
		ServerName: srvName,
		Def:        def,
		Manager:    mgr,
	}
}

func (t *MCPToolAdapter) Name() string {
	return fmt.Sprintf("%s_%s", t.ServerName, t.Def.Name)
}

func (t *MCPToolAdapter) Description() string {
	return fmt.Sprintf("[%s] %s", t.ServerName, t.Def.Description)
}

func (t *MCPToolAdapter) Capabilities() []Capability {
	return []Capability{CapNetwork, CapRead, CapWrite}
}

func (t *MCPToolAdapter) Definition() openrouter.Tool {
	schemaBytes, _ := json.Marshal(t.Def.InputSchema)

	return openrouter.Tool{
		Type: "function",
		Function: openrouter.ToolFunction{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  json.RawMessage(schemaBytes),
		},
	}
}

func (t *MCPToolAdapter) Execute(ctx context.Context, argsJSON string, rt *RuntimeContext) (string, error) {
	client, exists := t.Manager.Clients[t.ServerName]
	if !exists {
		return "", fmt.Errorf("MCP server '%s' is not connected", t.ServerName)
	}

	var argsMap map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &argsMap); err != nil {
		return "", fmt.Errorf("invalid arguments JSON: %w", err)
	}

	req := mcp_protocol.CallToolRequest{
		Params: mcp_protocol.CallToolParams{
			Name:      t.Def.Name,
			Arguments: argsMap,
		},
	}

	result, err := client.CallTool(ctx, req)
	if err != nil {
		return "", fmt.Errorf("remote MCP execution failed: %w", err)
	}

	if result.IsError {
		var errorText string
		for _, content := range result.Content {
			if tc, ok := content.(mcp_protocol.TextContent); ok {
				errorText += tc.Text + "\n"
			}
		}
		if errorText == "" {
			errorText = "Unknown tool error"
		}
		return fmt.Sprintf("MCP Error: %s", errorText), nil
	}

	var output string
	for _, content := range result.Content {
		switch c := content.(type) {
		case mcp_protocol.TextContent:
			output += c.Text + "\n"
		case mcp_protocol.ImageContent:
			output += fmt.Sprintf("[Image Content: %s (%s)]\n", c.Data, c.MIMEType)
		case mcp_protocol.EmbeddedResource:
			// FIX: Use %+v to print the resource safely, avoiding "URI undefined" errors
			// if the field name differs (e.g. Uri vs URI) in this version of the library.
			output += fmt.Sprintf("[Embedded Resource: %+v]\n", c.Resource)
		default:
			output += fmt.Sprintf("[Unknown Content Type: %T]\n", c)
		}
	}

	return output, nil
}
