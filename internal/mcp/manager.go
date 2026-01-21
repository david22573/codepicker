package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

type Manager struct {
	// FIX: Removed '*' because MCPClient is an interface
	Clients map[string]client.MCPClient
	Logger  logger.Logger
}

func NewManager(log logger.Logger) *Manager {
	return &Manager{
		// FIX: Update map initialization type here too
		Clients: make(map[string]client.MCPClient),
		Logger:  log,
	}
}

func (m *Manager) StartServers(ctx context.Context, configs []config.MCPServerConfig) {
	for _, cfg := range configs {
		m.Logger.Info(fmt.Sprintf("🔌 Connecting to MCP Server: %s (%s)", cfg.Name, cfg.Command))

		cli, err := client.NewStdioMCPClient(cfg.Command, cfg.Env, cfg.Args...)
		if err != nil {
			m.Logger.Error(fmt.Sprintf("Failed to create client for %s: %v", cfg.Name, err))
			continue
		}

		if err := cli.Start(ctx); err != nil {
			m.Logger.Error(fmt.Sprintf("Failed to start %s: %v", cfg.Name, err))
			continue
		}

		initReq := mcp.InitializeRequest{}
		initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
		initReq.Params.ClientInfo = mcp.Implementation{
			Name:    "codepicker",
			Version: "1.0.0",
		}
		initReq.Params.Capabilities = mcp.ClientCapabilities{}

		_, err = cli.Initialize(ctx, initReq)
		if err != nil {
			m.Logger.Error(fmt.Sprintf("Failed to initialize MCP handshake with %s: %v", cfg.Name, err))
			cli.Close()
			continue
		}

		m.Clients[cfg.Name] = cli
		m.Logger.Info(fmt.Sprintf("✅ MCP Connected: %s", cfg.Name))
	}
}

func (m *Manager) CloseAll() {
	for name, cli := range m.Clients {
		m.Logger.Debug(fmt.Sprintf("Stopping MCP server: %s", name))
		if err := cli.Close(); err != nil {
			m.Logger.Warn(fmt.Sprintf("Error closing %s: %v", name, err))
		}
	}
}

func (m *Manager) GetTools(ctx context.Context) ([]AvailableTool, error) {
	var allTools []AvailableTool

	for serverName, cli := range m.Clients {

		listCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		resp, err := cli.ListTools(listCtx, mcp.ListToolsRequest{})
		if err != nil {
			m.Logger.Warn(fmt.Sprintf("Failed to list tools from %s: %v", serverName, err))
			continue
		}

		for _, t := range resp.Tools {
			allTools = append(allTools, AvailableTool{
				ServerName: serverName,
				Tool:       t,
			})
		}
	}

	return allTools, nil
}

type AvailableTool struct {
	ServerName string
	Tool       mcp.Tool
}
