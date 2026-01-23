package database

import (
	"testing"
	"time"
)

func TestCheckpointSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Create a checkpoint
	cp := &Checkpoint{
		ID:        "test-checkpoint-1",
		SessionID: "session-1",
		PlanID:    "plan-1",
		Task:      "Test task",
		Timestamp: time.Now(),

		CurrentStep: 2,
		StepsStatus: map[int]string{
			1: "completed",
			2: "running",
			3: "pending",
		},
		StepResults: map[int]string{
			1: "First step done",
		},
		TurnCount:  10,
		ErrorCount: 1,
		LastError:  "Test error",
		Progress:   0.33,
		Status:     CheckpointActive,

		TotalCost:    0.15,
		RequestCount: 5,

		ApprovedWrite: true,
		ApprovedExec:  false,

		MemorySnapshot: &MemorySnapshot{
			Files: []MemoryFile{
				{Path: "test.go", Content: "package main", TokenCount: 10},
			},
		},

		ShadowFiles: map[string]string{
			"file1.go": "hash1",
			"file2.go": "hash2",
		},
		ShadowManifest: `{"version": 1}`,

		AgentModel:  "test-model",
		WorkerModel: "worker-model",
		PolicyName:  "batch",
		Metadata: map[string]string{
			"key1": "value1",
		},
	}

	// Save checkpoint
	if err := store.SaveCheckpoint(cp); err != nil {
		t.Fatalf("Failed to save checkpoint: %v", err)
	}

	// Load checkpoint
	loaded, err := store.LoadCheckpoint(cp.ID)
	if err != nil {
		t.Fatalf("Failed to load checkpoint: %v", err)
	}

	// Verify basic fields
	if loaded.ID != cp.ID {
		t.Errorf("Expected ID %s, got %s", cp.ID, loaded.ID)
	}
	if loaded.SessionID != cp.SessionID {
		t.Errorf("Expected SessionID %s, got %s", cp.SessionID, loaded.SessionID)
	}
	if loaded.PlanID != cp.PlanID {
		t.Errorf("Expected PlanID %s, got %s", cp.PlanID, loaded.PlanID)
	}
	if loaded.Task != cp.Task {
		t.Errorf("Expected Task %s, got %s", cp.Task, loaded.Task)
	}

	// Verify execution state
	if loaded.CurrentStep != cp.CurrentStep {
		t.Errorf("Expected CurrentStep %d, got %d", cp.CurrentStep, loaded.CurrentStep)
	}
	if loaded.TurnCount != cp.TurnCount {
		t.Errorf("Expected TurnCount %d, got %d", cp.TurnCount, loaded.TurnCount)
	}
	if loaded.Progress != cp.Progress {
		t.Errorf("Expected Progress %.2f, got %.2f", cp.Progress, loaded.Progress)
	}
	if loaded.Status != cp.Status {
		t.Errorf("Expected Status %s, got %s", cp.Status, loaded.Status)
	}

	// Verify steps status
	if len(loaded.StepsStatus) != len(cp.StepsStatus) {
		t.Errorf("Expected %d steps status, got %d", len(cp.StepsStatus), len(loaded.StepsStatus))
	}
	if loaded.StepsStatus[1] != "completed" {
		t.Errorf("Expected step 1 status 'completed', got %s", loaded.StepsStatus[1])
	}

	// Verify cost tracking
	if loaded.TotalCost != cp.TotalCost {
		t.Errorf("Expected TotalCost %.2f, got %.2f", cp.TotalCost, loaded.TotalCost)
	}
	if loaded.RequestCount != cp.RequestCount {
		t.Errorf("Expected RequestCount %d, got %d", cp.RequestCount, loaded.RequestCount)
	}

	// Verify approvals
	if loaded.ApprovedWrite != cp.ApprovedWrite {
		t.Errorf("Expected ApprovedWrite %v, got %v", cp.ApprovedWrite, loaded.ApprovedWrite)
	}
	if loaded.ApprovedExec != cp.ApprovedExec {
		t.Errorf("Expected ApprovedExec %v, got %v", cp.ApprovedExec, loaded.ApprovedExec)
	}

	// Verify memory snapshot
	if loaded.MemorySnapshot == nil {
		t.Fatal("Expected MemorySnapshot, got nil")
	}
	if len(loaded.MemorySnapshot.Files) != 1 {
		t.Errorf("Expected 1 file in memory, got %d", len(loaded.MemorySnapshot.Files))
	}

	// Verify shadow files
	if len(loaded.ShadowFiles) != 2 {
		t.Errorf("Expected 2 shadow files, got %d", len(loaded.ShadowFiles))
	}

	// Verify metadata
	if loaded.AgentModel != cp.AgentModel {
		t.Errorf("Expected AgentModel %s, got %s", cp.AgentModel, loaded.AgentModel)
	}
	if loaded.Metadata["key1"] != "value1" {
		t.Errorf("Expected metadata key1=value1, got %s", loaded.Metadata["key1"])
	}
}

func TestListCheckpoints(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	sessionID := "session-1"

	// Create multiple checkpoints
	for i := 0; i < 3; i++ {
		cp := &Checkpoint{
			ID:          string(rune('a' + i)),
			SessionID:   sessionID,
			Task:        "Test task",
			Timestamp:   time.Now().Add(time.Duration(i) * time.Second),
			CurrentStep: i,
			Progress:    float64(i) / 3.0,
			Status:      CheckpointActive,
			TotalCost:   float64(i) * 0.1,
			StepsStatus: map[int]string{},
			StepResults: map[int]string{},
			Metadata:    map[string]string{},
		}
		if err := store.SaveCheckpoint(cp); err != nil {
			t.Fatalf("Failed to save checkpoint %d: %v", i, err)
		}
	}

	// List checkpoints
	checkpoints, err := store.ListCheckpoints(sessionID)
	if err != nil {
		t.Fatalf("Failed to list checkpoints: %v", err)
	}

	if len(checkpoints) != 3 {
		t.Errorf("Expected 3 checkpoints, got %d", len(checkpoints))
	}

	// Verify ordering (should be descending by timestamp)
	for i := 0; i < len(checkpoints)-1; i++ {
		if checkpoints[i].Timestamp.Before(checkpoints[i+1].Timestamp) {
			t.Error("Checkpoints not ordered by timestamp (descending)")
		}
	}
}

func TestGetLatestCheckpoint(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	sessionID := "session-1"

	// Create checkpoints with different timestamps
	baseTime := time.Now()
	for i := 0; i < 3; i++ {
		cp := &Checkpoint{
			ID:          string(rune('a' + i)),
			SessionID:   sessionID,
			Task:        "Test task",
			Timestamp:   baseTime.Add(time.Duration(i) * time.Hour),
			CurrentStep: i,
			Status:      CheckpointActive,
			StepsStatus: map[int]string{},
			StepResults: map[int]string{},
			Metadata:    map[string]string{},
		}
		if err := store.SaveCheckpoint(cp); err != nil {
			t.Fatalf("Failed to save checkpoint %d: %v", i, err)
		}
	}

	// Get latest checkpoint
	latest, err := store.GetLatestCheckpoint(sessionID)
	if err != nil {
		t.Fatalf("Failed to get latest checkpoint: %v", err)
	}

	// Should be the last one (highest timestamp)
	if latest.ID != "c" {
		t.Errorf("Expected latest checkpoint ID 'c', got %s", latest.ID)
	}
	if latest.CurrentStep != 2 {
		t.Errorf("Expected latest checkpoint step 2, got %d", latest.CurrentStep)
	}
}

func TestUpdateCheckpointStatus(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	cp := &Checkpoint{
		ID:          "test-checkpoint",
		SessionID:   "session-1",
		Task:        "Test task",
		Timestamp:   time.Now(),
		Status:      CheckpointActive,
		StepsStatus: map[int]string{},
		StepResults: map[int]string{},
		Metadata:    map[string]string{},
	}

	if err := store.SaveCheckpoint(cp); err != nil {
		t.Fatalf("Failed to save checkpoint: %v", err)
	}

	// Update status
	if err := store.UpdateCheckpointStatus(cp.ID, CheckpointCompleted); err != nil {
		t.Fatalf("Failed to update status: %v", err)
	}

	// Verify update
	loaded, err := store.LoadCheckpoint(cp.ID)
	if err != nil {
		t.Fatalf("Failed to load checkpoint: %v", err)
	}

	if loaded.Status != CheckpointCompleted {
		t.Errorf("Expected status %s, got %s", CheckpointCompleted, loaded.Status)
	}
}

func TestDeleteCheckpoint(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	cp := &Checkpoint{
		ID:          "test-checkpoint",
		SessionID:   "session-1",
		Task:        "Test task",
		Timestamp:   time.Now(),
		Status:      CheckpointActive,
		StepsStatus: map[int]string{},
		StepResults: map[int]string{},
		Metadata:    map[string]string{},
	}

	if err := store.SaveCheckpoint(cp); err != nil {
		t.Fatalf("Failed to save checkpoint: %v", err)
	}

	// Delete checkpoint
	if err := store.DeleteCheckpoint(cp.ID); err != nil {
		t.Fatalf("Failed to delete checkpoint: %v", err)
	}

	// Verify deletion
	_, err = store.LoadCheckpoint(cp.ID)
	if err == nil {
		t.Error("Expected error when loading deleted checkpoint, got nil")
	}
}

func TestDeleteSessionCheckpoints(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	sessionID := "session-1"

	// Create multiple checkpoints
	for i := 0; i < 3; i++ {
		cp := &Checkpoint{
			ID:          string(rune('a' + i)),
			SessionID:   sessionID,
			Task:        "Test task",
			Timestamp:   time.Now(),
			Status:      CheckpointActive,
			StepsStatus: map[int]string{},
			StepResults: map[int]string{},
			Metadata:    map[string]string{},
		}
		if err := store.SaveCheckpoint(cp); err != nil {
			t.Fatalf("Failed to save checkpoint %d: %v", i, err)
		}
	}

	// Delete all session checkpoints
	if err := store.DeleteSessionCheckpoints(sessionID); err != nil {
		t.Fatalf("Failed to delete session checkpoints: %v", err)
	}

	// Verify deletion
	checkpoints, err := store.ListCheckpoints(sessionID)
	if err != nil {
		t.Fatalf("Failed to list checkpoints: %v", err)
	}

	if len(checkpoints) != 0 {
		t.Errorf("Expected 0 checkpoints after deletion, got %d", len(checkpoints))
	}
}

func TestGetAllSessions(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Create checkpoints for different sessions
	sessions := []string{"session-1", "session-2", "session-3"}
	for _, sessionID := range sessions {
		cp := &Checkpoint{
			ID:          sessionID + "-checkpoint",
			SessionID:   sessionID,
			Task:        "Test task",
			Timestamp:   time.Now(),
			Status:      CheckpointActive,
			StepsStatus: map[int]string{},
			StepResults: map[int]string{},
			Metadata:    map[string]string{},
		}
		if err := store.SaveCheckpoint(cp); err != nil {
			t.Fatalf("Failed to save checkpoint for %s: %v", sessionID, err)
		}
	}

	// Get all sessions
	allSessions, err := store.GetAllSessions()
	if err != nil {
		t.Fatalf("Failed to get all sessions: %v", err)
	}

	if len(allSessions) != 3 {
		t.Errorf("Expected 3 sessions, got %d", len(allSessions))
	}

	// Verify all sessions are present
	sessionMap := make(map[string]bool)
	for _, s := range allSessions {
		sessionMap[s] = true
	}

	for _, expected := range sessions {
		if !sessionMap[expected] {
			t.Errorf("Expected session %s not found in results", expected)
		}
	}
}

func TestCheckpointUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Create initial checkpoint
	cp := &Checkpoint{
		ID:          "test-checkpoint",
		SessionID:   "session-1",
		Task:        "Test task",
		Timestamp:   time.Now(),
		CurrentStep: 1,
		Progress:    0.33,
		Status:      CheckpointActive,
		StepsStatus: map[int]string{1: "completed"},
		StepResults: map[int]string{},
		Metadata:    map[string]string{},
	}

	if err := store.SaveCheckpoint(cp); err != nil {
		t.Fatalf("Failed to save checkpoint: %v", err)
	}

	// Update checkpoint
	cp.CurrentStep = 2
	cp.Progress = 0.66
	cp.StepsStatus[2] = "running"

	if err := store.SaveCheckpoint(cp); err != nil {
		t.Fatalf("Failed to update checkpoint: %v", err)
	}

	// Verify update
	loaded, err := store.LoadCheckpoint(cp.ID)
	if err != nil {
		t.Fatalf("Failed to load checkpoint: %v", err)
	}

	if loaded.CurrentStep != 2 {
		t.Errorf("Expected CurrentStep 2, got %d", loaded.CurrentStep)
	}
	if loaded.Progress != 0.66 {
		t.Errorf("Expected Progress 0.66, got %.2f", loaded.Progress)
	}
	if loaded.StepsStatus[2] != "running" {
		t.Errorf("Expected step 2 status 'running', got %s", loaded.StepsStatus[2])
	}
}
