package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/infra/fs"
	"github.com/david22573/codepicker/infra/hooks"
	"github.com/spf13/cobra"
)

var (
	formatFlag bool
)

var applyCmd = &cobra.Command{
	Use:   "apply [file_path]",
	Short: "Apply a file from shadow storage to the real filesystem",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		relPath := args[0]
		cwd, _ := os.Getwd()

		shadowMgr := fs.NewShadowManager(cwd)

		// 1. Validation & Summary (Existing Logic)
		shadowPath := filepath.Join(cwd, ".codepicker/shadow", relPath)
		if _, err := os.Stat(shadowPath); os.IsNotExist(err) {
			return fmt.Errorf("no shadow file found for %s", relPath)
		}

		summary, err := shadowMgr.Diff(relPath)
		if err != nil {
			return fmt.Errorf("failed to calculate diff: %w", err)
		}

		fmt.Println("---------------------------------------------------")
		fmt.Println("📝 PRE-APPLY CHANGE SUMMARY")
		fmt.Println(summary.String())
		fmt.Println("---------------------------------------------------")

		if summary.Type == fs.ChangeNoOp {
			fmt.Println("⚠️  No changes detected.")
			return nil
		}

		// 2. User Confirmation
		fmt.Print("Are you sure you want to apply these changes? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))

		if input != "y" && input != "yes" {
			fmt.Println("❌ Apply aborted.")
			return nil
		}

		// 3. Apply Changes
		fmt.Printf("Applying changes to %s...\n", relPath)
		if err := shadowMgr.Apply(relPath); err != nil {
			return fmt.Errorf("failed to apply: %w", err)
		}

		// 4. Post-Apply Hook: Formatting
		if formatFlag {
			realPath := filepath.Join(cwd, relPath)
			// Run formatting in background context
			if err := hooks.RunFormatter(context.Background(), realPath); err != nil {
				// Warn but do not fail the command, as the file is already applied
				fmt.Printf("⚠️  %v\n", err)
			}
		}

		fmt.Println("✅ Changes applied successfully.")
		return nil
	},
}

func init() {
	// Add opt-out flag for formatting
	applyCmd.Flags().BoolVar(&formatFlag, "fmt", true, "Run code formatter after applying changes")
	rootCmd.AddCommand(applyCmd)
}
