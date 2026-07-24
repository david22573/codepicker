package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	fixPlanOnly bool
	fixDryRun   bool
	fixApply    bool
	fixLlmModel string
	fixVerbose  bool
	fixBranch   bool
	fixResume   string
)

var fixCmd = &cobra.Command{
	Use:   "fix [file path]",
	Short: "Apply automated fixes to a file",
	Long: `Analyzes the target file for bugs, unsafe behavior, missing error handling, and applies automated fixes.
Wraps the unified agent engine with special Two-Pass Analysis.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		targetFile := args[0]
		taskDescription := fmt.Sprintf("Fix bugs, unsafe behavior, missing errors, and maintainability issues in %s", targetFile)

		opts := RunOptions{
			TaskDescription: taskDescription,
			TargetFile:      targetFile,
			PlanOnly:        fixPlanOnly,
			DryRun:          fixDryRun,
			Apply:           fixApply,
			Branch:          fixBranch,
			LlmModel:        fixLlmModel,
			ResumeSessionID: fixResume,
		}

		return RunTask(cmd.Context(), opts)
	},
}

func init() {
	fixCmd.Flags().BoolVar(&fixPlanOnly, "plan-only", false, "Generate plan, print details, but make zero changes")
	fixCmd.Flags().BoolVar(&fixDryRun, "dry-run", false, "Execute against shadow/sandbox only; make zero real changes")
	fixCmd.Flags().BoolVar(&fixApply, "apply", false, "Apply verified changes to the real filesystem")
	fixCmd.Flags().StringVar(&fixLlmModel, "model", "", "LLM model to use")
	fixCmd.Flags().BoolVarP(&fixVerbose, "verbose", "v", false, "Enable verbose output")
	fixCmd.Flags().BoolVarP(&fixBranch, "branch", "b", false, "Create a new git branch for this fix session")
	fixCmd.Flags().StringVar(&fixResume, "resume", "", "Resume a previous session by ID")
	rootCmd.AddCommand(fixCmd)
}
