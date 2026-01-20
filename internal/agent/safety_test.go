package agent

import (
	"context"
	"testing"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/policy"
	"github.com/david22573/codepicker/internal/tools"
	"github.com/david22573/codepicker/pkg/openrouter"
)

type mockShellTool struct{}

func (m *mockShellTool) Name() string        { return "run_shell" }
func (m *mockShellTool) Description() string { return "Run shell" }

// [Fixed] Return type must be openrouter.Tool, not any
func (m *mockShellTool) Definition() openrouter.Tool {
	return openrouter.Tool{}
}

func (m *mockShellTool) Execute(ctx context.Context, args string, rt *tools.RuntimeContext) (string, error) {
	return "", nil
}

func (m *mockShellTool) Capabilities() []tools.Capability {
	return []tools.Capability{tools.CapExecute}
}

func TestSafetyGuardrails(t *testing.T) {
	limits := config.DefaultLimits()
	sentinel := NewSentinel(limits)
	log := &logger.NoOpLogger{}

	enforcer := NewPolicyEnforcer(policy.Batch, log, sentinel, false)
	enforcer.RegisterTool(&mockShellTool{})

	tests := []struct {
		name        string
		tool        string
		args        string
		shouldAllow bool
	}{
		{"Block Shell", "run_shell", `{"command": "ls"}`, false},
		{"Block Dangerous", "run_shell", `{"command": "rm -rf /"}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := ApprovalRequest{Tool: tt.tool, Args: tt.args}
			if enforcer.AllowTool(req) != tt.shouldAllow {
				t.Errorf("got allow=%v, want %v", !tt.shouldAllow, tt.shouldAllow)
			}
		})
	}
}

func TestDebugWiring(t *testing.T) {
	enforcer := NewPolicyEnforcer(policy.Interactive, &logger.NoOpLogger{}, nil, true)
	if !enforcer.Debug {
		t.Error("Debug flag not propagated")
	}
}
