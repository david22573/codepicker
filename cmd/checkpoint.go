package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/david22573/codepicker/internal/database"
	"github.com/spf13/cobra"
)

var checkpointCmd = &cobra.Command{
	Use:   "checkpoint",
	Short: "Manage agent session checkpoints",
	Long:  `List, restore, and manage checkpoints for resumable agent sessions.`,
}

var listCheckpointsCmd = &cobra.Command{
	Use:   "list [session-id]",
	Short: "List all checkpoints for a session",
	Long:  `Lists all checkpoints for a specific session, or all sessions if no ID is provided.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := database.New(".codepicker")
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer store.Close()

		if len(args) == 0 {
			// List all sessions
			sessions, err := store.GetAllSessions()
			if err != nil {
				return fmt.Errorf("failed to list sessions: %w", err)
			}

			if len(sessions) == 0 {
				fmt.Println("No checkpoint sessions found.")
				return nil
			}

			fmt.Println("📂 Available Sessions:")
			fmt.Println()

			for _, sessionID := range sessions {
				checkpoints, err := store.ListCheckpoints(sessionID)
				if err != nil {
					continue
				}

				if len(checkpoints) == 0 {
					continue
				}

				latest := checkpoints[0]
				fmt.Printf("  Session: %s\n", sessionID)
				fmt.Printf("    Task: %s\n", latest.Task)
				fmt.Printf("    Checkpoints: %d\n", len(checkpoints))
				fmt.Printf("    Latest: %s (%.1f%% complete)\n", latest.Timestamp.Format("2006-01-02 15:04:05"), latest.Progress*100)
				fmt.Printf("    Status: %s\n", latest.Status)
				fmt.Printf("    Cost: $%.4f\n", latest.TotalCost)
				fmt.Println()
			}

			fmt.Println("Use 'codepicker checkpoint list <session-id>' to see detailed checkpoints.")
			return nil
		}

		// List checkpoints for specific session
		sessionID := args[0]
		checkpoints, err := store.ListCheckpoints(sessionID)
		if err != nil {
			return fmt.Errorf("failed to list checkpoints: %w", err)
		}

		if len(checkpoints) == 0 {
			fmt.Printf("No checkpoints found for session: %s\n", sessionID)
			return nil
		}

		fmt.Printf("📸 Checkpoints for Session: %s\n", sessionID)
		fmt.Printf("Task: %s\n\n", checkpoints[0].Task)

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tTimestamp\tProgress\tStep\tTurns\tCost\tStatus")
		fmt.Fprintln(w, "──\t─────────\t────────\t────\t─────\t────\t──────")

		for _, cp := range checkpoints {
			fmt.Fprintf(w, "%s\t%s\t%.1f%%\t%d\t%d\t$%.4f\t%s\n",
				cp.ID[:8]+"...",
				cp.Timestamp.Format("01-02 15:04"),
				cp.Progress*100,
				cp.CurrentStep,
				cp.TurnCount,
				cp.TotalCost,
				cp.Status,
			)
		}
		w.Flush()

		fmt.Println("\nUse 'codepicker checkpoint restore <checkpoint-id>' to resume from a checkpoint.")
		return nil
	},
}

var restoreCheckpointCmd = &cobra.Command{
	Use:   "restore <checkpoint-id>",
	Short: "Restore and resume from a checkpoint",
	Long:  `Restores the agent state from a checkpoint and resumes execution.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		checkpointID := args[0]

		store, err := database.New(".codepicker")
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer store.Close()

		// Load checkpoint to verify it exists
		checkpoint, err := store.LoadCheckpoint(checkpointID)
		if err != nil {
			return fmt.Errorf("checkpoint not found: %w", err)
		}

		fmt.Printf("📸 Restoring Checkpoint\n")
		fmt.Printf("   ID: %s\n", checkpoint.ID)
		fmt.Printf("   Session: %s\n", checkpoint.SessionID)
		fmt.Printf("   Task: %s\n", checkpoint.Task)
		fmt.Printf("   Progress: %.1f%% (Step %d)\n", checkpoint.Progress*100, checkpoint.CurrentStep)
		fmt.Printf("   Cost So Far: $%.4f (%d requests)\n", checkpoint.TotalCost, checkpoint.RequestCount)
		fmt.Printf("   Created: %s\n", checkpoint.Timestamp.Format("2006-01-02 15:04:05"))
		fmt.Println()

		// Note: Actual restoration would happen in the agent.go run command
		// This is just showing how to access the checkpoint data
		fmt.Println("✅ Checkpoint loaded successfully.")
		fmt.Println()
		fmt.Printf("To resume execution, use:\n")
		fmt.Printf("  codepicker agent resume %s\n", checkpoint.ID)

		return nil
	},
}

var cleanupCheckpointsCmd = &cobra.Command{
	Use:   "cleanup [session-id]",
	Short: "Clean up old checkpoints",
	Long:  `Removes checkpoints older than the specified age (default: 7 days).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		maxAge, _ := cmd.Flags().GetDuration("max-age")

		store, err := database.New(".codepicker")
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer store.Close()

		var sessions []string
		if len(args) > 0 {
			sessions = args
		} else {
			sessions, err = store.GetAllSessions()
			if err != nil {
				return fmt.Errorf("failed to list sessions: %w", err)
			}
		}

		totalDeleted := 0
		cutoff := time.Now().Add(-maxAge)

		for _, sessionID := range sessions {
			checkpoints, err := store.ListCheckpoints(sessionID)
			if err != nil {
				continue
			}

			for _, cp := range checkpoints {
				if cp.Timestamp.Before(cutoff) && cp.Status != database.CheckpointActive {
					if err := store.DeleteCheckpoint(cp.ID); err != nil {
						fmt.Printf("⚠️  Failed to delete checkpoint %s: %v\n", cp.ID[:8], err)
					} else {
						totalDeleted++
					}
				}
			}
		}

		fmt.Printf("🧹 Cleaned up %d old checkpoints (older than %s)\n", totalDeleted, maxAge)
		return nil
	},
}

var deleteCheckpointCmd = &cobra.Command{
	Use:   "delete <checkpoint-id|session-id>",
	Short: "Delete a checkpoint or all checkpoints for a session",
	Long:  `Deletes a specific checkpoint by ID, or all checkpoints for a session with --session flag.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		isSession, _ := cmd.Flags().GetBool("session")

		store, err := database.New(".codepicker")
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer store.Close()

		if isSession {
			// Delete all checkpoints for session
			checkpoints, err := store.ListCheckpoints(id)
			if err != nil {
				return fmt.Errorf("failed to list checkpoints: %w", err)
			}

			if len(checkpoints) == 0 {
				fmt.Printf("No checkpoints found for session: %s\n", id)
				return nil
			}

			fmt.Printf("⚠️  This will delete %d checkpoints for session %s\n", len(checkpoints), id)
			fmt.Print("Continue? [y/N]: ")

			var confirm string
			fmt.Scanln(&confirm)
			if confirm != "y" && confirm != "Y" {
				fmt.Println("Cancelled.")
				return nil
			}

			if err := store.DeleteSessionCheckpoints(id); err != nil {
				return fmt.Errorf("failed to delete checkpoints: %w", err)
			}

			fmt.Printf("✅ Deleted %d checkpoints\n", len(checkpoints))
		} else {
			// Delete single checkpoint
			if err := store.DeleteCheckpoint(id); err != nil {
				return fmt.Errorf("failed to delete checkpoint: %w", err)
			}
			fmt.Printf("✅ Deleted checkpoint: %s\n", id)
		}

		return nil
	},
}

func init() {
	checkpointCmd.AddCommand(listCheckpointsCmd)
	checkpointCmd.AddCommand(restoreCheckpointCmd)
	checkpointCmd.AddCommand(cleanupCheckpointsCmd)
	checkpointCmd.AddCommand(deleteCheckpointCmd)

	cleanupCheckpointsCmd.Flags().Duration("max-age", 7*24*time.Hour, "Maximum age of checkpoints to keep")
	deleteCheckpointCmd.Flags().Bool("session", false, "Delete all checkpoints for a session")

	rootCmd.AddCommand(checkpointCmd)
}
