package cmd

import (
	"fmt"
	"os"
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
	Run: func(cmd *cobra.Command, args []string) {
		opts := writer.TreeOptions{
			CopyToClipboard: treeCopy,
			OutPath:         treeOut,
		}
		w := writer.NewTreeStrategy(opts)

		// Get absolute source path
		absSrc, err := filepath.Abs(srcDir)
		if err != nil {
			logError(fmt.Sprintf("Invalid source path: %v", err))
			os.Exit(1)
		}

		logInfo(fmt.Sprintf("Generating tree for: %s", absSrc))
		runScan(w, absSrc)
	},
}

func init() {
	rootCmd.AddCommand(treeCmd)
	treeCmd.Flags().BoolVarP(&treeCopy, "copy", "c", false, "Copy tree output to clipboard")
	treeCmd.Flags().StringVarP(&treeOut, "out", "o", "", "Save tree output to a text file")
}
