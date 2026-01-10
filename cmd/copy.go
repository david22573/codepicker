package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/david22573/codepicker/internal/writer"
	"github.com/spf13/cobra"
)

var copyCmd = &cobra.Command{
	Use:   "copy",
	Short: "Copy files to a directory preserving structure",
	Run: func(cmd *cobra.Command, args []string) {
		// Validate output path
		absOut, err := filepath.Abs(outPath)
		if err != nil {
			logError(fmt.Sprintf("Invalid output path: %v", err))
			os.Exit(1)
		}

		// Check if user wants to overwrite source
		absSrc, _ := filepath.Abs(srcDir)
		if absSrc == absOut {
			logError("Cannot copy to source directory")
			os.Exit(1)
		}

		logInfo(fmt.Sprintf("Copy mode: output to %s", absOut))
		w := writer.NewCopyStrategy(absOut)
		runScan(w, absSrc)
	},
}

func init() {
	rootCmd.AddCommand(copyCmd)
	copyCmd.Flags().StringVarP(&outPath, "out", "o", "codepicker_out", "Output directory")
}
