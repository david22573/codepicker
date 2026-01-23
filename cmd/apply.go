package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/shadow"
	"github.com/spf13/cobra"
)

var applyCmd = &cobra.Command{
	Use:   "apply [file]",
	Short: "Apply staged changes from shadow to actual codebase",
	Long:  `Applies changes from the shadow directory to the source root. If a file argument is provided, only that file is applied. Otherwise, interactive mode is used.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		sm, err := shadow.NewManager(cfg.ProjectRoot)
		if err != nil {
			return err
		}

		if len(args) > 0 {
			relPath := args[0]
			return applyFile(sm, relPath)
		}

		changes := sm.GetManifestChanges()
		if len(changes) == 0 {
			fmt.Println("No pending changes in shadow.")
			return nil
		}

		fmt.Printf("Found %d pending changes:\n", len(changes))
		for path := range changes {
			if err := applyFile(sm, path); err != nil {
				// FIX: Use Sprintf because the interface only accepts a single string
				appLogger.Error(fmt.Sprintf("Failed to apply file %s: %v", path, err))
			}
		}

		return nil
	},
}

func applyFile(sm *shadow.Manager, relPath string) error {

	diff, err := sm.PreviewDiff(relPath)
	if err != nil {
		return fmt.Errorf("failed to generate diff for %s: %w", relPath, err)
	}

	fmt.Printf("\n--- Diff for %s ---\n%s\n", relPath, diff)
	fmt.Print("Apply this change? [y/N]: ")

	var response string
	fmt.Scanln(&response)

	if response != "y" && response != "Y" {
		fmt.Println("Skipping.")
		return nil
	}

	backup, err := sm.ApplyAtomic(relPath)
	if err != nil {
		return fmt.Errorf("failed to apply %s: %w", relPath, err)
	}

	fmt.Printf("✅ Applied %s (Backup saved to %s)\n", relPath, filepath.Base(backup))
	return nil
}

func init() {
	rootCmd.AddCommand(applyCmd)
}
