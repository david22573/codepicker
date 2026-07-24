package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/david22573/codepicker/infra/git"
	"github.com/spf13/cobra"
)

var (
	undoList bool
	undoLast bool
)

var undoCmd = &cobra.Command{
	Use:   "undo [run-id]",
	Short: "Rollback edits made by the CodePicker agent",
	Long: `Restores original file backups to revert the changes of past runs.
Supports:
  --list: lists all runs that are available to undo.
  --last: undoes the single most recent run.
  [run-id]: undoes the specific run by its ID.`,
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		var origStdout *os.File
		if GetJSON() {
			origStdout = os.Stdout
			os.Stdout = os.Stderr
			defer func() {
				os.Stdout = origStdout
				if err != nil {
					undoJSON := map[string]interface{}{
						"status": "fail",
						"error":  err.Error(),
					}
					jsonData, _ := json.Marshal(undoJSON)
					fmt.Fprintln(origStdout, string(jsonData))
				}
			}()
		}

		cwd, _ := os.Getwd()
		runsDir := filepath.Join(cwd, ".codepicker", "runs")

		// 1. List available runs
		runs, err := listRuns(runsDir)
		if err != nil {
			return err
		}

		if undoList {
			if len(runs) == 0 {
				fmt.Println("🤷 No undoable runs found.")
				if GetJSON() {
					os.Stdout = origStdout
					fmt.Println("[]")
				}
				return nil
			}
			if GetJSON() {
				os.Stdout = origStdout
				var list []map[string]interface{}
				for _, r := range runs {
					summaryPath := filepath.Join(runsDir, r, "summary.md")
					summaryContent, _ := os.ReadFile(summaryPath)
					statusLine := "UNKNOWN"
					lines := strings.Split(string(summaryContent), "\n")
					for _, line := range lines {
						if strings.HasPrefix(line, "Status:") || strings.HasPrefix(line, "## Status") {
							statusLine = strings.TrimSpace(line)
							statusLine = strings.TrimPrefix(statusLine, "Status:")
							statusLine = strings.TrimPrefix(statusLine, "## Status")
							statusLine = strings.TrimSpace(statusLine)
						}
					}
					list = append(list, map[string]interface{}{
						"run_id": r,
						"status": statusLine,
					})
				}
				jsonData, _ := json.MarshalIndent(list, "", "  ")
				fmt.Println(string(jsonData))
				return nil
			}
			fmt.Println("📜 Available runs to undo:")
			fmt.Println("---------------------------------------------------")
			for _, r := range runs {
				summaryPath := filepath.Join(runsDir, r, "summary.md")
				summaryContent, _ := os.ReadFile(summaryPath)
				statusLine := "UNKNOWN"
				lines := strings.Split(string(summaryContent), "\n")
				for _, line := range lines {
					if strings.HasPrefix(line, "Status:") || strings.HasPrefix(line, "## Status") {
						statusLine = strings.TrimSpace(line)
					}
				}
				fmt.Printf("   - ID: %s (%s)\n", r, statusLine)
			}
			fmt.Println("---------------------------------------------------")
			fmt.Println("To undo a run, call: codepicker undo <run-id>")
			return nil
		}

		var targetRun string
		if undoLast {
			if len(runs) == 0 {
				return fmt.Errorf("no past runs found to undo")
			}
			targetRun = runs[0] // newest run is first
		} else if len(args) == 1 {
			targetRun = args[0]
		} else {
			return fmt.Errorf("please specify a run-id, or use --last / --list")
		}

		// Verify target run exists
		targetPath := filepath.Join(runsDir, targetRun)
		if _, err := os.Stat(targetPath); os.IsNotExist(err) {
			return fmt.Errorf("run ID not found: %s", targetRun)
		}

		fmt.Printf("⏪ Rolling back changes from run: %s...\n", targetRun)

		// 2. Perform restore from backup directory
		backupDir := filepath.Join(targetPath, "backups")
		if _, err := os.Stat(backupDir); os.IsNotExist(err) {
			// Try checking general backups folder or git fallback
			fmt.Printf("⚠️  No backup files found for %s. Reverting files listed in patch...\n", targetRun)
		} else {
			files, err := os.ReadDir(backupDir)
			if err == nil && len(files) > 0 {
				var restoredFiles []string
				for _, file := range files {
					if file.IsDir() {
						continue
					}
					backupFile := filepath.Join(backupDir, file.Name())
					realFile := filepath.Join(cwd, file.Name()) // standard root fallback or recreate

					// Find actual path by walking or mapping
					// If we saved it flat, we copy it back
					data, err := os.ReadFile(backupFile)
					if err == nil {
						if err := os.WriteFile(realFile, data, 0644); err != nil {
							fmt.Printf("❌ Failed to restore %s: %v\n", file.Name(), err)
						} else {
							fmt.Printf("✅ Restored original: %s\n", file.Name())
							restoredFiles = append(restoredFiles, file.Name())
						}
					}
				}
				fmt.Println("🎉 Rollback complete using backup files!")
				if GetJSON() {
					os.Stdout = origStdout
					undoJSON := map[string]interface{}{
						"status":       "pass",
						"run_id":       targetRun,
						"files_undone": restoredFiles,
					}
					jsonData, _ := json.MarshalIndent(undoJSON, "", "  ")
					fmt.Println(string(jsonData))
				}
				return nil
			}
		}

		// 3. Fallback: Parse patch.diff to revert files from git status if safe
		patchPath := filepath.Join(targetPath, "patch.diff")
		if data, err := os.ReadFile(patchPath); err == nil {
			patchContent := string(data)
			lines := strings.Split(patchContent, "\n")
			var filesToCheckout []string
			for _, line := range lines {
				if strings.HasPrefix(line, "+++ b/") {
					f := strings.TrimPrefix(line, "+++ b/")
					filesToCheckout = append(filesToCheckout, f)
				}
			}

			if len(filesToCheckout) > 0 {
				fmt.Printf("🌿 Fallback: checking out %d files from Git HEAD...\n", len(filesToCheckout))
				gitClient := git.NewClient(cwd, false)
				if err := gitClient.RevertFiles(cmd.Context(), filesToCheckout); err != nil {
					return fmt.Errorf("git checkout fallback failed: %w", err)
				}
				fmt.Println("🎉 Git checkout rollback complete!")
				if GetJSON() {
					os.Stdout = origStdout
					undoJSON := map[string]interface{}{
						"status":       "pass",
						"run_id":       targetRun,
						"files_undone": filesToCheckout,
					}
					jsonData, _ := json.MarshalIndent(undoJSON, "", "  ")
					fmt.Println(string(jsonData))
				}
				return nil
			}
		}

		return fmt.Errorf("no rollback backups or patches found for %s", targetRun)
	},
}

func listRuns(runsDir string) ([]string, error) {
	if _, err := os.Stat(runsDir); os.IsNotExist(err) {
		return nil, nil
	}

	files, err := os.ReadDir(runsDir)
	if err != nil {
		return nil, err
	}

	var runs []string
	for _, file := range files {
		if file.IsDir() && (strings.HasPrefix(file.Name(), "run-") || strings.HasPrefix(file.Name(), "sess_")) {
			runs = append(runs, file.Name())
		}
	}

	// Sort runs newest first (runID format run-YYYYMMDD-HHMMSS sorts chronologically)
	sort.Slice(runs, func(i, j int) bool {
		return runs[i] > runs[j]
	})

	return runs, nil
}

func init() {
	undoCmd.Flags().BoolVar(&undoList, "list", false, "List all past runs available to undo")
	undoCmd.Flags().BoolVar(&undoLast, "last", false, "Undo the single most recent run")
	rootCmd.AddCommand(undoCmd)
}
