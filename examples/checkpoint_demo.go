package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/david22573/codepicker/internal/agent"
	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/database"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/pkg/openrouter"
	"github.com/google/uuid"
)

// This example demonstrates the checkpoint system for resumable agent sessions
func main() {
	fmt.Println("🔄 Checkpoint System Demo")
	fmt.Println("=" + string(make([]byte, 50)))
	fmt.Println()

	// Check for API key
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENROUTER_API_KEY environment variable not set")
	}

	// Setup
	tmpDir, err := os.MkdirTemp("", "checkpoint-demo-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	fmt.Printf("📁 Working directory: %s\n\n", tmpDir)

	// Initialize components
	store, err := database.New(tmpDir)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	client := openrouter.NewClient(apiKey)
	log := logger.NewStandardLogger(1)
	limits := config.DefaultLimits()

	engine, err := agent.NewEngine(
		client,
		"deepseek/deepseek-chat",
		tmpDir,
		log,
		limits,
		store,
		nil,
		agent.DebugConfig{},
	)
	if err != nil {
		log.Fatal(err)
	}

	// Demo 1: Create a plan with multiple steps
	fmt.Println("📋 Demo 1: Creating a multi-step plan")
	plan := &agent.Plan{
		ID:           uuid.New().String(),
		OriginalTask: "Demonstrate checkpoint system",
		Steps: []agent.Step{
			{
				ID:          1,
				Description: "Initialize demo environment",
				Instruction: "Create a simple Go file with package main",
				Status:      "pending",
			},
			{
				ID:          2,
				Description: "Add functionality",
				Instruction: "Add a function that demonstrates checkpointing",
				Status:      "pending",
			},
			{
				ID:          3,
				Description: "Add documentation",
				Instruction: "Add comments explaining the checkpoint system",
				Status:      "pending",
			},
		},
		EstimatedCost: 0.05,
	}

	// Save the plan
	if err := store.SavePlan(plan.ID, plan.OriginalTask, plan.Steps, plan.EstimatedCost); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("✅ Created plan with %d steps\n", len(plan.Steps))
	fmt.Printf("   Plan ID: %s\n\n", plan.ID)

	// Demo 2: Create executor with checkpointing enabled
	fmt.Println("📸 Demo 2: Creating executor with auto-checkpointing")
	executor := agent.NewPlanExecutor(engine, plan)
	executor.AutoCheckpoint = true
	executor.CheckpointInterval = 1 // Checkpoint after every step

	sessionID := executor.CheckpointManager.SessionID
	fmt.Printf("✅ Executor created with session ID: %s\n", sessionID)
	fmt.Printf("   Auto-checkpoint: enabled\n")
	fmt.Printf("   Checkpoint interval: every %d step(s)\n\n", executor.CheckpointInterval)

	// Demo 3: Create manual checkpoint before execution
	fmt.Println("📸 Demo 3: Creating initial checkpoint")
	ctx := context.Background()

	checkpoint1, err := executor.CheckpointManager.CreateCheckpoint(ctx, plan, 0)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("✅ Initial checkpoint created\n")
	fmt.Printf("   Checkpoint ID: %s\n", checkpoint1.ID)
	fmt.Printf("   Progress: %.1f%%\n", checkpoint1.Progress*100)
	fmt.Printf("   Status: %s\n\n", checkpoint1.Status)

	// Demo 4: Simulate partial execution
	fmt.Println("🏃 Demo 4: Simulating partial execution")

	// Mark first step as completed
	plan.Steps[0].Status = "completed"
	plan.Steps[0].Result = "Demo file created successfully"

	// Mark second step as running
	plan.Steps[1].Status = "running"

	// Create checkpoint after first step
	checkpoint2, err := executor.CheckpointManager.CreateCheckpoint(ctx, plan, 1)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("✅ Checkpoint after step 1\n")
	fmt.Printf("   Checkpoint ID: %s\n", checkpoint2.ID)
	fmt.Printf("   Progress: %.1f%%\n", checkpoint2.Progress*100)
	fmt.Printf("   Current step: %d\n\n", checkpoint2.CurrentStep)

	// Demo 5: List all checkpoints
	fmt.Println("📋 Demo 5: Listing all checkpoints for session")

	checkpoints, err := executor.CheckpointManager.ListCheckpoints()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Found %d checkpoints:\n\n", len(checkpoints))
	for i, cp := range checkpoints {
		fmt.Printf("  %d. ID: %s\n", i+1, cp.ID[:12]+"...")
		fmt.Printf("     Timestamp: %s\n", cp.Timestamp.Format("15:04:05"))
		fmt.Printf("     Progress: %.1f%%\n", cp.Progress*100)
		fmt.Printf("     Step: %d\n", cp.CurrentStep)
		fmt.Printf("     Status: %s\n\n", cp.Status)
	}

	// Demo 6: Restore from checkpoint
	fmt.Println("🔄 Demo 6: Restoring from first checkpoint")

	// Create a new engine to simulate fresh start
	engine2, err := agent.NewEngine(
		client,
		"deepseek/deepseek-chat",
		tmpDir,
		log,
		limits,
		store,
		nil,
		agent.DebugConfig{},
	)
	if err != nil {
		log.Fatal(err)
	}

	// Create new checkpoint manager (simulating new session)
	newCM := agent.NewCheckpointManager(store, "new-session", engine2)

	// Restore from first checkpoint
	restoredPlan, currentStep, err := newCM.RestoreCheckpoint(ctx, checkpoint1.ID)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("✅ Checkpoint restored successfully\n")
	fmt.Printf("   Restored session ID: %s\n", newCM.SessionID)
	fmt.Printf("   Plan ID: %s\n", restoredPlan.ID)
	fmt.Printf("   Current step: %d\n", currentStep)
	fmt.Printf("   Total steps: %d\n\n", len(restoredPlan.Steps))

	// Demo 7: Get latest checkpoint
	fmt.Println("📸 Demo 7: Getting latest checkpoint")

	latestCP, err := executor.CheckpointManager.GetLatestCheckpoint()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("✅ Latest checkpoint:\n")
	fmt.Printf("   ID: %s\n", latestCP.ID[:12]+"...")
	fmt.Printf("   Created: %s\n", latestCP.Timestamp.Format("15:04:05"))
	fmt.Printf("   Progress: %.1f%%\n", latestCP.Progress*100)
	fmt.Printf("   Current step: %d/%d\n\n", latestCP.CurrentStep, len(plan.Steps))

	// Demo 8: Update checkpoint status
	fmt.Println("📝 Demo 8: Updating checkpoint status")

	if err := store.UpdateCheckpointStatus(checkpoint1.ID, database.CheckpointPaused); err != nil {
		log.Fatal(err)
	}

	updated, err := store.LoadCheckpoint(checkpoint1.ID)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("✅ Checkpoint status updated\n")
	fmt.Printf("   Old status: %s\n", checkpoint1.Status)
	fmt.Printf("   New status: %s\n\n", updated.Status)

	// Demo 9: Cleanup old checkpoints
	fmt.Println("🧹 Demo 9: Cleaning up old checkpoints")

	// Create an old checkpoint by modifying timestamp
	oldCheckpoint := &database.Checkpoint{
		ID:          uuid.New().String(),
		SessionID:   sessionID,
		Task:        "Old task",
		Timestamp:   time.Now().Add(-48 * time.Hour),
		Status:      database.CheckpointCompleted,
		StepsStatus: map[int]string{},
		StepResults: map[int]string{},
		Metadata:    map[string]string{},
	}

	if err := store.SaveCheckpoint(oldCheckpoint); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Created old checkpoint (48 hours ago): %s\n", oldCheckpoint.ID[:12]+"...")

	// Clean up checkpoints older than 24 hours
	if err := executor.CheckpointManager.CleanupOldCheckpoints(24 * time.Hour); err != nil {
		log.Fatal(err)
	}

	// Verify cleanup
	remainingCPs, err := executor.CheckpointManager.ListCheckpoints()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("✅ Cleanup complete\n")
	fmt.Printf("   Remaining checkpoints: %d\n\n", len(remainingCPs))

	// Demo 10: Delete specific checkpoint
	fmt.Println("🗑️  Demo 10: Deleting specific checkpoint")

	if err := store.DeleteCheckpoint(checkpoint1.ID); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("✅ Deleted checkpoint: %s\n\n", checkpoint1.ID[:12]+"...")

	// Demo 11: Get all sessions
	fmt.Println("📂 Demo 11: Getting all sessions")

	sessions, err := store.GetAllSessions()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Found %d session(s):\n", len(sessions))
	for i, sess := range sessions {
		cps, _ := store.ListCheckpoints(sess)
		fmt.Printf("  %d. Session: %s\n", i+1, sess[:12]+"...")
		fmt.Printf("     Checkpoints: %d\n", len(cps))
	}
	fmt.Println()

	// Summary
	fmt.Println("=" + string(make([]byte, 50)))
	fmt.Println("✨ Demo Complete!")
	fmt.Println()
	fmt.Println("Key Features Demonstrated:")
	fmt.Println("  ✓ Automatic checkpoint creation")
	fmt.Println("  ✓ Manual checkpoint creation")
	fmt.Println("  ✓ Checkpoint listing and querying")
	fmt.Println("  ✓ State restoration from checkpoint")
	fmt.Println("  ✓ Checkpoint status updates")
	fmt.Println("  ✓ Old checkpoint cleanup")
	fmt.Println("  ✓ Session management")
	fmt.Println()
	fmt.Println("Next Steps:")
	fmt.Println("  • Try: codepicker checkpoint list")
	fmt.Println("  • Try: codepicker agent resume <checkpoint-id>")
	fmt.Println("  • Try: codepicker checkpoint cleanup --max-age 24h")
}
