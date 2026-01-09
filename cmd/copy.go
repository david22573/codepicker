package cmd

import (
	"path/filepath"

	"github.com/david22573/codepicker/internal/writer"
	"github.com/spf13/cobra"
)

var copyCmd = &cobra.Command{
	Use:   "copy",
	Short: "Copy files to a directory preserving structure",
	Run: func(cmd *cobra.Command, args []string) {
		finalOut := outPath
		if finalOut == "codepicker_context.txt" {
			finalOut = "codepicker_out"
		}
		absOut, _ := filepath.Abs(finalOut)

		w := writer.NewCopyStrategy(absOut)
		runScan(w)
	},
}

func init() {
	rootCmd.AddCommand(copyCmd)
	copyCmd.Flags().StringVarP(&outPath, "out", "o", "codepicker_out", "Output directory")
}

