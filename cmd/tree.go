package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/david22573/codepicker/internal/writer"
	"github.com/spf13/cobra"
)

var (
	treeCopy bool
	treeOut  string
)

var treeCmd = &cobra.Command{
	Use:   "tree",
	Short: "Print a visual tree of the project",
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := writer.TreeOptions{
			CopyToClipboard: treeCopy,
			OutPath:         treeOut,
		}
		w := writer.NewTreeStrategy(opts)

		absSrc, err := filepath.Abs(srcDir)
		if err != nil {
			return fmt.Errorf("invalid source path: %w", err)
		}

		appLogger.Info(fmt.Sprintf("Generating tree for: %s", absSrc))

		// Updated: Pass 'cmd'
		return runScan(cmd.Context(), w, absSrc, cmd)
	},
}

func init() {
	rootCmd.AddCommand(treeCmd)
	treeCmd.Flags().BoolVarP(&treeCopy, "copy", "C", false, "Copy tree output to clipboard")
	treeCmd.Flags().StringVarP(&treeOut, "out", "o", "", "Save tree output to a text file")
}
