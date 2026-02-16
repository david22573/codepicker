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
	Run: func(cmd *cobra.Command, args []string) {
		// Get project root
		wd, _ := os.Getwd()

		// Initialize ShadowManager directly
		// We default DryRun to false here because 'apply' is an explicit user action
		shadowMgr := fs.NewShadowManager(wd, false)

		// Determine which files to apply
		var filesToApply []string

		if len(args) > 0 {
			// User specified a specific file
			filesToApply = append(filesToApply, args[0])
		} else {
			// Auto-discover all pending files
			pending, err := shadowMgr.ListShadowFiles()
			if err != nil {
				fmt.Printf("❌ Failed to list shadow files: %v\n", err)
				os.Exit(1)
			}
			if len(pending) == 0 {
				fmt.Println("✨ No pending shadow changes found.")
				return
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
			fmt.Printf("⚠️  Applied %d/%d files. Check errors above.\n", successCount, len(filesToApply))
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(applyCmd)
}
