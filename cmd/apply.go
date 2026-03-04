package cmd

import (
	"fmt"
	"os"

	"github.com/david22573/codepicker/infra/fs"
	"github.com/spf13/cobra"
)

var applyCmd = &cobra.Command{
	Use:   "apply [file]",
	Short: "Apply pending shadow changes to the real filesystem",
	Long: `Moves files from the shadow storage (.codepicker/shadow) to the actual project structure.
If a filename is provided, only that file is applied.
If no argument is provided, ALL pending shadow files are applied.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		wd, _ := os.Getwd()

		shadowMgr := fs.NewShadowManager(wd, false)

		var filesToApply []string

		if len(args) > 0 {
			filesToApply = append(filesToApply, args[0])
		} else {
			pending, err := shadowMgr.ListShadowFiles()
			if err != nil {
				return fmt.Errorf("failed to list shadow files: %w", err)
			}
			if len(pending) == 0 {
				fmt.Println("✨ No pending shadow changes found.")
				return nil
			}
			filesToApply = pending
		}

		fmt.Printf("📦 Applying %d file(s) to project...\n", len(filesToApply))

		successCount := 0
		for _, file := range filesToApply {
			err := shadowMgr.Commit(file)
			if err != nil {
				fmt.Printf("❌ Failed to apply '%s': %v\n", file, err)
			} else {
				fmt.Printf("✅ Applied: %s\n", file)
				successCount++
			}
		}

		if successCount == len(filesToApply) {
			fmt.Println("🎉 All changes applied successfully.")
		} else {
			return fmt.Errorf("applied %d/%d files with errors", successCount, len(filesToApply))
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(applyCmd)
}
