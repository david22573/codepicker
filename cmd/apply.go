package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/david22573/codepicker/infra/fs"
	"github.com/spf13/cobra"
)

var applyCmd = &cobra.Command{
	Use:   "apply [file_path]",
	Short: "Apply a file from shadow storage to the real filesystem",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		relPath := args[0]
		cwd, _ := os.Getwd()

		shadowMgr := fs.NewShadowManager(cwd)

		fmt.Printf("Applying changes to %s...\n", relPath)

		shadowPath := filepath.Join(cwd, ".codepicker/shadow", relPath)
		if _, err := os.Stat(shadowPath); os.IsNotExist(err) {
			return fmt.Errorf("no shadow file found for %s", relPath)
		}

		if err := shadowMgr.Apply(relPath); err != nil {
			return fmt.Errorf("failed to apply: %w", err)
		}

		fmt.Println("✅ Changes applied successfully.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(applyCmd)
}
