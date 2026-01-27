package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/david22573/codepicker/infra/hooks"
	"github.com/spf13/cobra"
)

var fmtCmd = &cobra.Command{
	Use:   "fmt [file]",
	Short: "Run the configured formatter on a file (manual trigger)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := args[0]
		absPath, _ := filepath.Abs(path)

		// Verify file exists
		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			return fmt.Errorf("file not found: %s", path)
		}

		fmt.Printf("Running formatter on %s...\n", path)
		return hooks.RunFormatter(context.Background(), absPath)
	},
}

func init() {
	rootCmd.AddCommand(fmtCmd)
}
