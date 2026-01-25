package database_test

import (
	"testing"
	"time"

	"github.com/david22573/codepicker/internal/agent"
	"github.com/david22573/codepicker/internal/database"
)

func newTestStore(t *testing.T) *database.Store {
	// Using :memory: for creating a fresh, isolated DB for each test
	// Note: In your real implementation, New() expects a directory path.
	// We might need to modify New() or use a temp dir.
	// Assuming New() creates a directory:
	tmpDir := t.TempDir()
	store, err := database.New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create test store: %v", err)
	}
	return store
}

func TestPlanPersistence(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	planID := "test-plan-1"
	task := "Fix the bug"
	steps := []agent.Step{
		{ID: 1, Description: "Analyze", Status: "pending"},
	}
	estCost := 0.50

	// Save
	if err := store.SavePlan(planID, task, steps, estCost); err != nil {
		t.Fatalf("SavePlan failed: %v", err)
	}

	// Load
	plan, err := store.GetPlan(planID)
	if err != nil {
		t.Fatalf("GetPlan failed: %v", err)
	}

	if plan.Task != task {
		t.Errorf("Task mismatch: got %s, want %s", plan.Task, task)
	}
	if plan.EstimatedCost != estCost {
		t.Errorf("Cost mismatch: got %f, want %f", plan.EstimatedCost, estCost)
	}

	// Update Status
	if err := store.UpdatePlanStatus(planID, "completed"); err != nil {
		t.Fatalf("UpdatePlanStatus failed: %v", err)
	}

	plan, _ = store.GetPlan(planID)
	if plan.Status != "completed" {
		t.Errorf("Status mismatch: got %s, want completed", plan.Status)
	}
}

func TestCheckpointing(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	cp := &database.Checkpoint{
		ID:           "cp-1",
		SessionID:    "session-1",
		Task:         "My Task",
		Timestamp:    time.Now(),
		CurrentStep:  1,
		Status:       database.CheckpointActive,
		StepsStatus:  map[int]string{1: "completed"},
		StepResults:  map[int]string{1: "OK"},
		ShadowFiles:  map[string]string{"main.go": "hash123"},
		TotalCost:    0.1,
		RequestCount: 5,
	}

	if err := store.SaveCheckpoint(cp); err != nil {
		t.Fatalf("SaveCheckpoint failed: %v", err)
	}

	loaded, err := store.LoadCheckpoint("cp-1")
	if err != nil {
		t.Fatalf("LoadCheckpoint failed: %v", err)
	}

	if loaded.SessionID != cp.SessionID {
		t.Errorf("SessionID mismatch")
	}
	if len(loaded.StepsStatus) != 1 {
		t.Errorf("StepsStatus map lost data")
	}
	if loaded.TotalCost != 0.1 {
		t.Errorf("TotalCost mismatch")
	}

	// List Checkpoints
	list, err := store.ListCheckpoints("session-1")
	if err != nil {
		t.Fatalf("ListCheckpoints failed: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("Expected 1 checkpoint, got %d", len(list))
	}
}

func TestMemoryOperations(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	// Add file
	if err := store.UpdateWorkingMemory("main.go", "package main"); err != nil {
		t.Fatalf("UpdateWorkingMemory failed: %v", err)
	}

	// Retrieve
	ctxStr, tokens, err := store.GetWorkingMemory()
	if err != nil {
		t.Fatalf("GetWorkingMemory failed: %v", err)
	}
	if tokens == 0 {
		t.Error("Expected non-zero tokens")
	}
	if ctxStr == "" {
		t.Error("Expected context string, got empty")
	}

	// List
	files, err := store.ListMemoryFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != "main.go" {
		t.Errorf("ListMemoryFiles mismatch: %v", files)
	}

	// Remove
	if err := store.RemoveFromMemory("main.go"); err != nil {
		t.Fatal(err)
	}
	files, _ = store.ListMemoryFiles()
	if len(files) != 0 {
		t.Error("File was not removed")
	}
}
