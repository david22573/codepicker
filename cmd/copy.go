package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/david22573/codepicker/internal/writer"
	"github.com/spf13/cobra"
)

var copyCmd = &cobra.Command{
	Use:   "copy",
	Short: "Copy files to a directory preserving structure",
	RunE: func(cmd *cobra.Command, args []string) error {
		absOut, err := filepath.Abs(outPath)
		if err != nil {
			return fmt.Errorf("invalid output path: %w", err)
		}

		absSrc, err := filepath.Abs(srcDir)
		if err != nil {
			return fmt.Errorf("invalid source directory: %w", err)
		}

		if absSrc == absOut {
			return fmt.Errorf("cannot copy to source directory")
		}

		appLogger.Info(fmt.Sprintf("Copy mode: output to %s", absOut))
		w := writer.NewCopyStrategy(absOut)

		// Updated: Pass 'cmd'
		return runScan(cmd.Context(), w, absSrc, cmd)
	},
}

func init() {
	rootCmd.AddCommand(copyCmd)
	copyCmd.Flags().StringVarP(&outPath, "out", "o", "codepicker_out", "Output directory")
}

