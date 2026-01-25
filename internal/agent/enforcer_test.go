package agent_test

import (
	"testing"
	"time"

	"github.com/david22573/codepicker/internal/agent"
	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/policy"
	"github.com/david22573/codepicker/internal/tools"
)

// Mock tool to register with capabilities
type mockWriteTool struct{}

func (m *mockWriteTool) Name() string                     { return "write_file" }
func (m *mockWriteTool) Description() string              { return "writes" }
func (m *mockWriteTool) Capabilities() []tools.Capability { return []tools.Capability{tools.CapWrite} }
func (m *mockWriteTool) Definition() interface{}          { return nil } // simplified
func (m *mockWriteTool) Execute()                         {}

func TestPolicyEnforcer(t *testing.T) {
	log := &logger.NoOpLogger{}
	limits := &config.Limits{CommandTimeout: time.Second}
	sentinel := agent.NewSentinel(limits)

	t.Run("Batch Mode Blocks Shell", func(t *testing.T) {
		// Batch policy has AllowShell = false
		enforcer := agent.NewPolicyEnforcer(policy.Batch, log, sentinel, false)

		// We must register the tool or it will be "Unknown"
		// Manual registration for test since we don't have the full registry here easily
		enforcer.ToolCaps = map[string][]tools.Capability{
			"run_shell": {tools.CapExecute},
		}

		req := agent.ApprovalRequest{
			Tool: "run_shell",
			Args: `{"command": "ls"}`,
		}

		if allowed := enforcer.AllowTool(req); allowed {
			t.Error("Batch policy should block shell, but it allowed it")
		}
	})

	t.Run("Interactive Mode Prompts", func(t *testing.T) {
		enforcer := agent.NewPolicyEnforcer(policy.Interactive, log, sentinel, false)
		enforcer.ToolCaps = map[string][]tools.Capability{
			"write_file": {tools.CapWrite},
		}

		req := agent.ApprovalRequest{
			Tool: "write_file",
			Args: `{"path": "test.go"}`,
		}

		// Mock the interaction handler
		called := false
		enforcer.SetInteractionHandler(func(r agent.ApprovalRequest) agent.ApprovalResponse {
			called = true
			return agent.ApprovalResponse{Approved: true}
		})

		if allowed := enforcer.AllowTool(req); !allowed {
			t.Error("Expected tool to be allowed by mock handler")
		}
		if !called {
			t.Error("Interaction handler was not called")
		}
	})

	t.Run("Session Scope Approval", func(t *testing.T) {
		enforcer := agent.NewPolicyEnforcer(policy.Interactive, log, sentinel, false)
		enforcer.ToolCaps = map[string][]tools.Capability{
			"write_file": {tools.CapWrite},
		}

		req := agent.ApprovalRequest{Tool: "write_file"}

		// First call: Approve with Session Scope
		enforcer.SetInteractionHandler(func(r agent.ApprovalRequest) agent.ApprovalResponse {
			return agent.ApprovalResponse{Approved: true, SessionScope: true}
		})

		enforcer.AllowTool(req)

		if !enforcer.Session.AllowWrite {
			t.Error("Session.AllowWrite should be true after session scope approval")
		}

		// Second call: Handler forces false, but session should override it
		enforcer.SetInteractionHandler(func(r agent.ApprovalRequest) agent.ApprovalResponse {
			return agent.ApprovalResponse{Approved: false}
		})

		if allowed := enforcer.AllowTool(req); !allowed {
			t.Error("Tool should be auto-allowed due to session scope, but was blocked")
		}
	})
}
