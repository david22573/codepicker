package agent

import (
	"context"
	"testing"
	"time"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/database"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/pkg/openrouter"
	"github.com/google/uuid"
)

func TestCheckpointCreation(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := database.New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	client := openrouter.NewClient("fake-api-key")
	log := &logger.NoOpLogger{}
	limits := config.DefaultLimits()

	engine, err := NewEngine(client, "test-model", tmpDir, log, limits, store, nil, DebugConfig{})
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	sessionID := uuid.New().String()
	cm := NewCheckpointManager(store, sessionID, engine)

	// Create a test plan
	plan := &Plan{
		ID:           uuid.New().String(),
		OriginalTask: "Test task",
		Steps: []Step{
			{ID: 1, Description: "Step 1", Status: "completed", Result: "Done"},
			{ID: 2, Description: "Step 2", Status: "running"},
			{ID: 3, Description: "Step 3", Status: "pending"},
		},
		EstimatedCost: 0.10,
	}

	// Create checkpoint
	ctx := context.Background()
	checkpoint, err := cm.CreateCheckpoint(ctx, plan, 1)
	if err != nil {
		t.Fatalf("Failed to create checkpoint: %v", err)
	}

	// Verify checkpoint
	if checkpoint.SessionID != sessionID {
		t.Errorf("Expected session ID %s, got %s", sessionID, checkpoint.SessionID)
	}
	if checkpoint.PlanID != plan.ID {
		t.Errorf("Expected plan ID %s, got %s", plan.ID, checkpoint.PlanID)
	}
	if checkpoint.Task != plan.OriginalTask {
		t.Errorf("Expected task %s, got %s", plan.OriginalTask, checkpoint.Task)
	}
	if checkpoint.CurrentStep != 1 {
		t.Errorf("Expected current step 1, got %d", checkpoint.CurrentStep)
	}

	// Verify step statuses
	if len(checkpoint.StepsStatus) != 3 {
		t.Errorf("Expected 3 step statuses, got %d", len(checkpoint.StepsStatus))
	}
	if checkpoint.StepsStatus[1] != "completed" {
		t.Errorf("Expected step 1 status 'completed', got %s", checkpoint.StepsStatus[1])
	}

	// Verify progress calculation
	expectedProgress := 1.0 / 3.0
	if checkpoint.Progress < expectedProgress-0.01 || checkpoint.Progress > expectedProgress+0.01 {
		t.Errorf("Expected progress ~%.2f, got %.2f", expectedProgress, checkpoint.Progress)
	}
}

func TestCheckpointRestore(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := database.New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	client := openrouter.NewClient("fake-api-key")
	log := &logger.NoOpLogger{}
	limits := config.DefaultLimits()

	engine, err := NewEngine(client, "test-model", tmpDir, log, limits, store, nil, DebugConfig{})
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	sessionID := uuid.New().String()
	cm := NewCheckpointManager(store, sessionID, engine)

	// Create and save a plan
	plan := &Plan{
		ID:           uuid.New().String(),
		OriginalTask: "Test restoration",
		Steps: []Step{
			{ID: 1, Description: "Step 1", Status: "completed", Result: "Done"},
			{ID: 2, Description: "Step 2", Status: "running"},
			{ID: 3, Description: "Step 3", Status: "pending"},
		},
		EstimatedCost: 0.15,
	}

	// Save plan to store
	if err := store.SavePlan(plan.ID, plan.OriginalTask, plan.Steps, plan.EstimatedCost); err != nil {
		t.Fatalf("Failed to save plan: %v", err)
	}

	// Create checkpoint
	ctx := context.Background()
	checkpoint, err := cm.CreateCheckpoint(ctx, plan, 1)
	if err != nil {
		t.Fatalf("Failed to create checkpoint: %v", err)
	}

	// Create new engine and checkpoint manager for restoration
	engine2, err := NewEngine(client, "test-model", tmpDir, log, limits, store, nil, DebugConfig{})
	if err != nil {
		t.Fatalf("Failed to create second engine: %v", err)
	}

	cm2 := NewCheckpointManager(store, "new-session", engine2)

	// Restore checkpoint
	restoredPlan, currentStep, err := cm2.RestoreCheckpoint(ctx, checkpoint.ID)
	if err != nil {
		t.Fatalf("Failed to restore checkpoint: %v", err)
	}

	// Verify restoration
	if restoredPlan.ID != plan.ID {
		t.Errorf("Expected plan ID %s, got %s", plan.ID, restoredPlan.ID)
	}
	if currentStep != 1 {
		t.Errorf("Expected current step 1, got %d", currentStep)
	}
	if len(restoredPlan.Steps) != 3 {
		t.Errorf("Expected 3 steps, got %d", len(restoredPlan.Steps))
	}
	if restoredPlan.Steps[0].Status != "completed" {
		t.Errorf("Expected step 1 status 'completed', got %s", restoredPlan.Steps[0].Status)
	}

	// Verify session ID was updated
	if cm2.SessionID != sessionID {
		t.Errorf("Expected session ID to be updated to %s, got %s", sessionID, cm2.SessionID)
	}
}

func TestCheckpointListing(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := database.New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	client := openrouter.NewClient("fake-api-key")
	log := &logger.NoOpLogger{}
	limits := config.DefaultLimits()

	engine, err := NewEngine(client, "test-model", tmpDir, log, limits, store, nil, DebugConfig{})
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	sessionID := uuid.New().String()
	cm := NewCheckpointManager(store, sessionID, engine)

	plan := &Plan{
		ID:           uuid.New().String(),
		OriginalTask: "Test listing",
		Steps: []Step{
			{ID: 1, Description: "Step 1", Status: "pending"},
		},
		EstimatedCost: 0.05,
	}

	// Create multiple checkpoints
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_, err := cm.CreateCheckpoint(ctx, plan, i)
		if err != nil {
			t.Fatalf("Failed to create checkpoint %d: %v", i, err)
		}
		time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	}

	// List checkpoints
	checkpoints, err := cm.ListCheckpoints()
	if err != nil {
		t.Fatalf("Failed to list checkpoints: %v", err)
	}

	if len(checkpoints) != 3 {
		t.Errorf("Expected 3 checkpoints, got %d", len(checkpoints))
	}

	// Verify they're ordered by timestamp (descending)
	for i := 0; i < len(checkpoints)-1; i++ {
		if checkpoints[i].Timestamp.Before(checkpoints[i+1].Timestamp) {
			t.Error("Checkpoints are not ordered by timestamp (descending)")
		}
	}
}

func TestCheckpointCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := database.New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	client := openrouter.NewClient("fake-api-key")
	log := &logger.TestLogger{}
	limits := config.DefaultLimits()

	engine, err := NewEngine(client, "test-model", tmpDir, log, limits, store, nil, DebugConfig{})
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	sessionID := uuid.New().String()
	cm := NewCheckpointManager(store, sessionID, engine)

	plan := &Plan{
		ID:            uuid.New().String(),
		OriginalTask:  "Test cleanup",
		Steps:         []Step{{ID: 1, Description: "Step 1", Status: "completed"}},
		EstimatedCost: 0.05,
	}

	ctx := context.Background()

	// Create an old checkpoint
	checkpoint1, err := cm.CreateCheckpoint(ctx, plan, 0)
	if err != nil {
		t.Fatalf("Failed to create checkpoint: %v", err)
	}

	// Mark it as completed and make it appear old
	checkpoint1.Status = database.CheckpointCompleted
	checkpoint1.Timestamp = time.Now().Add(-48 * time.Hour)
	if err := store.SaveCheckpoint(checkpoint1); err != nil {
		t.Fatalf("Failed to update checkpoint: %v", err)
	}

	// Create a recent checkpoint
	_, err = cm.CreateCheckpoint(ctx, plan, 1)
	if err != nil {
		t.Fatalf("Failed to create second checkpoint: %v", err)
	}

	// Cleanup old checkpoints (older than 24 hours)
	if err := cm.CleanupOldCheckpoints(24 * time.Hour); err != nil {
		t.Fatalf("Failed to cleanup: %v", err)
	}

	// Verify only the recent checkpoint remains
	checkpoints, err := cm.ListCheckpoints()
	if err != nil {
		t.Fatalf("Failed to list checkpoints: %v", err)
	}

	if len(checkpoints) != 1 {
		t.Errorf("Expected 1 checkpoint after cleanup, got %d", len(checkpoints))
	}
}

func TestMemorySnapshotInCheckpoint(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := database.New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	client := openrouter.NewClient("fake-api-key")
	log := &logger.NoOpLogger{}
	limits := config.DefaultLimits()

	engine, err := NewEngine(client, "test-model", tmpDir, log, limits, store, nil, DebugConfig{})
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	// Add some files to memory
	if err := engine.Memory.Store.UpdateWorkingMemory("test.go", "package main\n"); err != nil {
		t.Fatalf("Failed to add to memory: %v", err)
	}

	sessionID := uuid.New().String()
	cm := NewCheckpointManager(store, sessionID, engine)

	plan := &Plan{
		ID:            uuid.New().String(),
		OriginalTask:  "Test memory snapshot",
		Steps:         []Step{{ID: 1, Description: "Step 1", Status: "running"}},
		EstimatedCost: 0.05,
	}

	// Create checkpoint
	ctx := context.Background()
	checkpoint, err := cm.CreateCheckpoint(ctx, plan, 0)
	if err != nil {
		t.Fatalf("Failed to create checkpoint: %v", err)
	}

	// Verify memory snapshot
	if checkpoint.MemorySnapshot == nil {
		t.Fatal("Expected memory snapshot in checkpoint")
	}
	if len(checkpoint.MemorySnapshot.Files) != 1 {
		t.Errorf("Expected 1 file in memory snapshot, got %d", len(checkpoint.MemorySnapshot.Files))
	}
	if checkpoint.MemorySnapshot.Files[0].Path != "test.go" {
		t.Errorf("Expected file 'test.go', got '%s'", checkpoint.MemorySnapshot.Files[0].Path)
	}
}

func TestSessionApprovalRestoration(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := database.New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	client := openrouter.NewClient("fake-api-key")
	log := &logger.NoOpLogger{}
	limits := config.DefaultLimits()

	engine, err := NewEngine(client, "test-model", tmpDir, log, limits, store, nil, DebugConfig{})
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	// Set session approvals
	engine.Enforcer.Session.AllowWrite = true
	engine.Enforcer.Session.AllowExec = true

	sessionID := uuid.New().String()
	cm := NewCheckpointManager(store, sessionID, engine)

	plan := &Plan{
		ID:            uuid.New().String(),
		OriginalTask:  "Test approvals",
		Steps:         []Step{{ID: 1, Description: "Step 1", Status: "running"}},
		EstimatedCost: 0.05,
	}

	// Create checkpoint
	ctx := context.Background()
	checkpoint, err := cm.CreateCheckpoint(ctx, plan, 0)
	if err != nil {
		t.Fatalf("Failed to create checkpoint: %v", err)
	}

	// Verify approvals were saved
	if !checkpoint.ApprovedWrite {
		t.Error("Expected ApprovedWrite to be true")
	}
	if !checkpoint.ApprovedExec {
		t.Error("Expected ApprovedExec to be true")
	}

	// Create new engine with no approvals
	engine2, err := NewEngine(client, "test-model", tmpDir, log, limits, store, nil, DebugConfig{})
	if err != nil {
		t.Fatalf("Failed to create second engine: %v", err)
	}

	cm2 := NewCheckpointManager(store, "new-session", engine2)

	// Restore checkpoint
	_, _, err = cm2.RestoreCheckpoint(ctx, checkpoint.ID)
	if err != nil {
		t.Fatalf("Failed to restore checkpoint: %v", err)
	}

	// Verify approvals were restored
	if !engine2.Enforcer.Session.AllowWrite {
		t.Error("Expected AllowWrite to be restored to true")
	}
	if !engine2.Enforcer.Session.AllowExec {
		t.Error("Expected AllowExec to be restored to true")
	}
}
