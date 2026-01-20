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

	// FIX: Access fields via RuntimeContext
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
}

func TestEngineCallback(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := database.New(tmpDir)
	defer store.Close()

	client := openrouter.NewClient("key")

	eng, _ := NewEngine(client, "model", tmpDir, &logger.NoOpLogger{}, config.DefaultLimits(), store, nil)

	if eng.Enforcer.Check("rm -rf /", "dangerous") {
		t.Error("Default policy should deny permission for rm -rf")
	}

	eng.Enforcer.Policy = policy.Interactive

	if eng.Enforcer.Check("ls", "listing") {
		t.Error("Default interaction handler should deny permission when in interactive mode")
	}

	eng.Enforcer.OnApproval = func(c, r string) bool { return true }
	if !eng.Enforcer.Check("ls", "listing") {
		t.Error("Overridden callback should allow permission")
	}
}
