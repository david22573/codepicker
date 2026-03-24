package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/david22573/codepicker/infra/git"
	"github.com/spf13/cobra"
)

var undoCmd = &cobra.Command{
	Use:   "undo [N]",
	Short: "Revert the last N commits made by the CodePicker agent",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()
		gitClient := git.NewClient(cwd, false)

		count := 1
		if len(args) == 1 {
			parsed, err := strconv.Atoi(args[0])
			if err != nil || parsed < 1 {
				return fmt.Errorf("invalid number of commits to undo: %s", args[0])
			}
			count = parsed
		}

		fmt.Printf("🔍 Looking for the last %d [codepicker] commit(s)...\n", count)
		hashes, err := gitClient.GetLastCodepickerCommits(cmd.Context(), count)
		if err != nil {
			return fmt.Errorf("failed to fetch commits: %w", err)
		}

		if len(hashes) == 0 {
			fmt.Println("🤷 No recent agent commits found to undo.")
			return nil
		}

		fmt.Printf("⏪ Reverting %d commit(s)...\n", len(hashes))
		for _, hash := range hashes {
			fmt.Printf("   - Reverting %s\n", hash[:8])
		}

		if err := gitClient.RevertCommits(cmd.Context(), hashes); err != nil {
			return fmt.Errorf("failed to revert commits: %w", err)
		}

		fmt.Println("✅ Undo complete! Changes have been safely reverted.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(undoCmd)
}
