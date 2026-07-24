package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	runPlanOnly     bool
	runDryRun       bool
	runApply        bool
	runCiMode       bool
	runLlmModel     string
	runVerbose      bool
	runNoMap        bool
	runNoAutoCommit bool
	runBranch       bool
	runResume       string
	runForce        bool
)

var runCmd = &cobra.Command{
	Use:   "run [task description]",
	Short: "Run a single task using the agent",
	Long: `Executes a natural language coding instruction using the ReAct agent.
Supports multiple safety modes:
  --plan-only: generates the plan but makes no changes.
  --dry-run: generates and executes against sandbox/shadow filesystem only.
  --apply: applies verified changes to the real project filesystem.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		taskDescription := ""
		if len(args) > 0 {
			taskDescription = args[0]
		}

		if taskDescription == "" && runResume == "" {
			return fmt.Errorf("task description is required if not resuming a session")
		}

		opts := RunOptions{
			TaskDescription: taskDescription,
			PlanOnly:        runPlanOnly,
			DryRun:          runDryRun,
			Apply:           runApply,
			Branch:          runBranch,
			CiMode:          runCiMode,
			LlmModel:        runLlmModel,
			NoMap:           runNoMap,
			NoAutoCommit:    runNoAutoCommit,
			ResumeSessionID: runResume,
			Force:           runForce,
		}

		return RunTask(cmd.Context(), opts)
	},
}

func init() {
	runCmd.Flags().BoolVar(&runPlanOnly, "plan-only", false, "Generate plan, print details, but make zero changes")
	runCmd.Flags().BoolVar(&runDryRun, "dry-run", false, "Execute against shadow/sandbox only; make zero real changes")
	runCmd.Flags().BoolVar(&runApply, "apply", false, "Apply verified changes to the real filesystem (requires confirmation)")
	runCmd.Flags().BoolVar(&runCiMode, "ci", false, "Enable CI mode (skip confirmations)")
	runCmd.Flags().StringVar(&runLlmModel, "model", "", "LLM model to use")
	runCmd.Flags().BoolVarP(&runVerbose, "verbose", "v", false, "Enable verbose output")
	runCmd.Flags().BoolVar(&runNoMap, "no-map", false, "Disable the sparse repository map injection")
	runCmd.Flags().BoolVar(&runNoAutoCommit, "no-autocommit", false, "Disable auto-committing successful file edits")
	runCmd.Flags().BoolVarP(&runBranch, "branch", "b", false, "Create a new git branch for this session")
	runCmd.Flags().StringVar(&runResume, "resume", "", "Resume a previous session by ID")
	runCmd.Flags().BoolVar(&runForce, "force", false, "Force apply even if verifier fails")
	rootCmd.AddCommand(runCmd)
}
