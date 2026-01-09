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
		// outPath defaults to "codepicker_out" via the flag definition in init() below
		absOut, _ := filepath.Abs(outPath)
		w := writer.NewCopyStrategy(absOut)
		runScan(w)
	},
}

func init() {
	rootCmd.AddCommand(copyCmd)
	// We set a specific default for the copy command here
	copyCmd.Flags().StringVarP(&outPath, "out", "o", "codepicker_out", "Output directory")
}

