package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/david22573/codepicker/internal/database"
	"github.com/google/uuid"
)

func main() {
	// Create a temporary database location for the demo
	tmpDir, _ := os.MkdirTemp("", "codepicker_demo")
	defer os.RemoveAll(tmpDir)

	fmt.Println("🚀 Starting Checkpoint Demo...")

	// Initialize the store
	store, err := database.New(tmpDir)
	if err != nil {
		log.Fatalf("Failed to init db: %v", err)
	}
	defer store.Close()

	// 1. Create a dummy session and checkpoint
	sessionID := uuid.New().String()
	checkpointID := uuid.New().String()

	cp := &database.Checkpoint{
		ID:          checkpointID,
		SessionID:   sessionID,
		Task:        "Refactor Login Page",
		Timestamp:   time.Now(),
		CurrentStep: 2,
		Status:      database.CheckpointActive,
		TotalCost:   0.15,
		StepsStatus: map[int]string{1: "completed", 2: "running"},
		StepResults: map[int]string{1: "Login form updated"},
	}

	fmt.Printf("💾 Saving checkpoint for session %s...\n", sessionID[:8])
	if err := store.SaveCheckpoint(cp); err != nil {
		log.Fatalf("Failed to save: %v", err)
	}

	// 2. List checkpoints
	fmt.Println("\n📋 Listing checkpoints:")
	checkpoints, err := store.ListCheckpoints(sessionID)
	if err != nil {
		log.Fatal(err)
	}

	for _, c := range checkpoints {
		fmt.Printf("   - [%s] Step %d: %s ($%.2f)\n",
			c.Timestamp.Format("15:04:05"),
			c.CurrentStep,
			c.Status,
			c.TotalCost,
		)
	}

	// 3. Load full data
	fmt.Println("\n🔄 Reloading full data...")
	loaded, err := store.LoadCheckpoint(checkpointID)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("   Successfully loaded task: %q\n", loaded.Task)

	fmt.Println("\n✅ Demo Complete.")
}
