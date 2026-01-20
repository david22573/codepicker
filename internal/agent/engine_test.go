package agent

import (
	"testing"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/database"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/policy"
	"github.com/david22573/codepicker/pkg/openrouter"
)

func TestNewEngine(t *testing.T) {
	tmpDir := t.TempDir()

	store, err := database.New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create test db: %v", err)
	}
	defer store.Close()

	client := openrouter.NewClient("fake-api-key")
	limits := config.DefaultLimits()
	log := &logger.NoOpLogger{}

	engine, err := NewEngine(client, "test-model", tmpDir, log, limits, store, nil)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	if engine.Model != "test-model" {
		t.Errorf("Expected model 'test-model', got %s", engine.Model)
	}

	if engine.Executor == nil {
		t.Fatal("Engine executor not initialized")
	}

	rt := engine.Executor.RuntimeContext
	if rt == nil {
		t.Fatal("Runtime context not initialized")
	}

	if rt.Memory == nil {
		t.Error("Engine working memory not initialized")
	}

	if rt.FS == nil {
		t.Error("Engine virtual filesystem not initialized")
	}

	if engine.Sentinel == nil {
		t.Error("Engine sentinel not initialized")
	}

	if engine.Enforcer == nil {
		t.Error("Engine enforcer not initialized")
	}
}

func TestEngineCallback(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := database.New(tmpDir)
	defer store.Close()

	client := openrouter.NewClient("key")

	eng, _ := NewEngine(client, "model", tmpDir, &logger.NoOpLogger{}, config.DefaultLimits(), store, nil)

	// Test 1: Default Policy (Batch) should block shell by default
	// Batch policy sets AllowShell=false
	req := ApprovalRequest{
		Tool: "run_shell",
		Args: `{"command": "rm -rf /"}`,
	}

	if eng.Enforcer.AllowTool(req) {
		t.Error("Default policy (Batch) should deny permission for shell access")
	}

	// Test 2: Interactive Policy requiring handler
	eng.Enforcer.Policy = policy.Interactive

	// No handler set yet -> should fail safely
	eng.Enforcer.OnApproval = nil
	reqInteractive := ApprovalRequest{
		Tool: "run_shell",
		Args: `{"command": "ls"}`,
	}
	if eng.Enforcer.AllowTool(reqInteractive) {
		t.Error("Should deny permission when interactive handler is missing")
	}

	// Test 3: Handler set -> should use return value
	eng.Enforcer.OnApproval = func(r ApprovalRequest) bool { return true }
	if !eng.Enforcer.AllowTool(reqInteractive) {
		t.Error("Overridden callback should allow permission")
	}
}
