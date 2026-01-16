package agent

import (
	"testing"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/database"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/pkg/openrouter"
)

func TestNewEngine(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize temp database for test
	store, err := database.New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create test db: %v", err)
	}
	defer store.Close()

	client := openrouter.NewClient("fake-api-key")
	limits := config.DefaultLimits()
	log := &logger.NoOpLogger{}

	engine, err := NewEngine(client, "test-model", tmpDir, log, limits, store)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	if engine.Model != "test-model" {
		t.Errorf("Expected model 'test-model', got %s", engine.Model)
	}

	if engine.Memory == nil {
		t.Error("Engine working memory not initialized")
	}

	if engine.Shadow == nil {
		t.Error("Engine shadow manager not initialized")
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
	eng, _ := NewEngine(client, "model", tmpDir, &logger.NoOpLogger{}, config.DefaultLimits(), store)

	if eng.ApprovalCallback("rm -rf /", "dangerous") {
		t.Error("Default callback should deny permission")
	}

	eng.ApprovalCallback = func(c, r string) bool { return true }
	if !eng.ApprovalCallback("ls", "listing") {
		t.Error("Overridden callback should allow permission")
	}
}
