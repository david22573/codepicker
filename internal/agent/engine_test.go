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
	store, _ := database.New(tmpDir)
	defer store.Close()

	client := openrouter.NewClient("fake-api-key")
	limits := config.DefaultLimits()
	log := &logger.NoOpLogger{}

	// [Fixed] Pass DebugConfig{}
	engine, err := NewEngine(client, "test-model", tmpDir, log, limits, store, nil, DebugConfig{})
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	if engine.Executor == nil {
		t.Fatal("Engine executor not initialized")
	}
	if engine.Sentinel == nil {
		t.Error("Engine sentinel not initialized")
	}
}
