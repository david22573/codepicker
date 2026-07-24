package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/app"
	"github.com/spf13/cobra"
)

var (
	applyYes   bool
	applyForce bool
)

var applyCmd = &cobra.Command{
	Use:   "apply [plan-id | patch-file]",
	Short: "Apply verified shadow changes or patch files to the real filesystem",
	Long: `Applies verified plans, patch files, or pending shadow files to the real filesystem.
Previews files to be changed and asks for confirmation before making changes.
Creates transaction backups under '.codepicker/backups/' automatically for rollback support.`,
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		var origStdout *os.File
		if GetJSON() {
			origStdout = os.Stdout
			os.Stdout = os.Stderr
			defer func() {
				os.Stdout = origStdout
				if err != nil {
					applyJSON := map[string]interface{}{
						"status": "fail",
						"error":  err.Error(),
					}
					jsonData, _ := json.Marshal(applyJSON)
					fmt.Fprintln(origStdout, string(jsonData))
				}
			}()
		}

		cwd, _ := os.Getwd()
		container, err := app.NewContainer("", cwd, "", false, false, GetVerbose())
		if err != nil {
			return fmt.Errorf("failed to initialize: %w", err)
		}
		defer container.Close()

		var patchContent string
		var shadowFiles []string

		if len(args) > 0 {
			target := args[0]

			if strings.HasSuffix(target, ".diff") || strings.HasSuffix(target, ".patch") || strings.Contains(target, "/") || strings.Contains(target, "\\") {
				// 1. Target is a patch file path
				data, err := os.ReadFile(target)
				if err != nil {
					// Check if it exists under .codepicker/runs/
					runsPath := filepath.Join(cwd, ".codepicker", "runs", target, "patch.diff")
					data, err = os.ReadFile(runsPath)
					if err != nil {
						return fmt.Errorf("failed to read patch file %s: %w", target, err)
					}
				}
				patchContent = string(data)
				// Extract file paths from unified diff format (lines starting with +++ b/path)
				lines := strings.Split(patchContent, "\n")
				for _, line := range lines {
					if strings.HasPrefix(line, "+++ b/") {
						f := strings.TrimPrefix(line, "+++ b/")
						shadowFiles = append(shadowFiles, f)
					}
				}
			} else {
				// 2. Target is a plan ID
				plan, err := container.Repository.GetPlan(cmd.Context(), target)
				if err != nil {
					// Fallback to checking runs folder
					runsPath := filepath.Join(cwd, ".codepicker", "runs", target, "patch.diff")
					data, err := os.ReadFile(runsPath)
					if err == nil {
						patchContent = string(data)
						lines := strings.Split(patchContent, "\n")
						for _, line := range lines {
							if strings.HasPrefix(line, "+++ b/") {
								f := strings.TrimPrefix(line, "+++ b/")
								shadowFiles = append(shadowFiles, f)
							}
						}
					} else {
						return fmt.Errorf("plan or run ID not found: %s", target)
					}
				} else {
					// We have a SQLite plan
					// Try to load patch from its run directory if exists
					runsPath := filepath.Join(cwd, ".codepicker", "runs", plan.ID, "patch.diff")
					data, err := os.ReadFile(runsPath)
					if err == nil {
						patchContent = string(data)
						lines := strings.Split(patchContent, "\n")
						for _, line := range lines {
							if strings.HasPrefix(line, "+++ b/") {
								f := strings.TrimPrefix(line, "+++ b/")
								shadowFiles = append(shadowFiles, f)
							}
						}
					}
				}
			}
		} else {
			// 3. Apply all pending shadow changes
			pending, err := container.ShadowManager.ListShadowFiles()
			if err != nil {
				return fmt.Errorf("failed to list shadow files: %w", err)
			}
			if len(pending) == 0 {
				fmt.Println("✨ No pending shadow changes found.")
				if GetJSON() {
					os.Stdout = origStdout
					applyJSON := map[string]interface{}{
						"status":        "pass",
						"message":       "No pending shadow changes found.",
						"files_applied": []string{},
					}
					jsonData, _ := json.MarshalIndent(applyJSON, "", "  ")
					fmt.Println(string(jsonData))
				}
				return nil
			}
			shadowFiles = pending
		}

		if len(shadowFiles) == 0 {
			fmt.Println("🤷 No files found to change.")
			if GetJSON() {
				os.Stdout = origStdout
				applyJSON := map[string]interface{}{
					"status":        "pass",
					"message":       "No files found to change.",
					"files_applied": []string{},
				}
				jsonData, _ := json.MarshalIndent(applyJSON, "", "  ")
				fmt.Println(string(jsonData))
			}
			return nil
		}

		// Preview changes
		fmt.Println("\nFiles to change:")
		for _, f := range shadowFiles {
			fmt.Printf("- [MODIFY] %s\n", f)
		}

		// Sandbox Verification
		if patchContent != "" && !applyForce {
			fmt.Println("🧪 Verifying patch in sandbox...")
			verifyRes, err := container.Verifier.Verify(cmd.Context(), patchContent)
			if err != nil {
				return fmt.Errorf("verification error: %w", err)
			}
			if !verifyRes.Success {
				fmt.Printf("\n❌ Verifier FAILED at stage '%s'.\n", verifyRes.Stage)
				fmt.Println(verifyRes.Logs)
				fmt.Println("\n⚠️  Apply blocked. Use --force to override.")
				return fmt.Errorf("verification failed")
			}
			fmt.Println("✅ Verification passed.")
		}

		// Prompt user if --yes is not set
		proceed := applyYes
		if !proceed {
			fmt.Print("\nProceed? [y/N]: ")
			reader := bufio.NewReader(os.Stdin)
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(strings.ToLower(input))
			proceed = (input == "y" || input == "yes")
		}

		if !proceed {
			fmt.Println("❌ Cancelled.")
			if GetJSON() {
				os.Stdout = origStdout
				applyJSON := map[string]interface{}{
					"status": "cancelled",
					"error":  "apply cancelled by user",
				}
				jsonData, _ := json.MarshalIndent(applyJSON, "", "  ")
				fmt.Println(string(jsonData))
			}
			return nil
		}

		// Start Transaction for Backup/Rollback
		tx, err := container.WorkspaceManager.BeginTransaction()
		if err != nil {
			return fmt.Errorf("failed to start transaction: %w", err)
		}
		defer tx.Rollback()

		// Backup files before editing
		for _, f := range shadowFiles {
			_ = tx.BackupFile(f)
		}

		fmt.Printf("📦 Applying changes to project...\n")

		if patchContent != "" {
			// Apply using verified blocks
			if err := container.Verifier.ApplyToReal(patchContent); err != nil {
				return fmt.Errorf("apply failed: %w", err)
			}
			fmt.Printf("✅ Applied patch (%d files updated)\n", len(shadowFiles))
		} else {
			// Apply from shadow manager
			successCount := 0
			for _, file := range shadowFiles {
				err := container.ShadowManager.Commit(file)
				if err != nil {
					fmt.Printf("❌ Failed to apply '%s': %v\n", file, err)
				} else {
					fmt.Printf("✅ Applied: %s\n", file)
					successCount++
				}
			}
			if successCount != len(shadowFiles) {
				return fmt.Errorf("applied %d/%d files with errors", successCount, len(shadowFiles))
			}
		}

		_ = tx.Commit()
		fmt.Println("🎉 All changes applied successfully.")

		if GetJSON() {
			os.Stdout = origStdout
			applyJSON := map[string]interface{}{
				"status":        "pass",
				"files_applied": shadowFiles,
			}
			jsonData, _ := json.MarshalIndent(applyJSON, "", "  ")
			fmt.Println(string(jsonData))
		}

		return nil
	},
}

func init() {
	applyCmd.Flags().BoolVar(&applyYes, "yes", false, "Skip confirmation prompt")
	applyCmd.Flags().BoolVar(&applyForce, "force", false, "Force apply even if verifier fails")
	rootCmd.AddCommand(applyCmd)
}
