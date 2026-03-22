package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/david22573/codepicker/app"
	"github.com/david22573/codepicker/domain/task"
	"github.com/david22573/codepicker/infra/git"
	"github.com/spf13/cobra"
)

var runDryRun bool
var runCiMode bool
var runLlmModel string
var runVerbose bool
var runNoMap bool
var runNoAutoCommit bool
var runBranch bool

var runCmd = &cobra.Command{
	Use:   "run [task description]",
	Short: "Run a single task using the agent",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiKey := getAPIKeyOrExit()
		taskDescription := args[0]
		cwd, _ := os.Getwd()

		if runNoAutoCommit {
			os.Setenv("CODEPICKER_NO_AUTOCOMMIT", "1")
		}

		gitClient := git.NewClient(cwd, runDryRun)
		var branchName string
		if runBranch {
			slug := slugifyTask(taskDescription)
			branchName = fmt.Sprintf("cp/%d-%s", time.Now().Unix(), slug)
			if err := gitClient.CreateBranch(branchName); err != nil {
				fmt.Printf("⚠️  Failed to create session branch: %v\n", err)
			} else {
				fmt.Printf("🌿 Switched to new session branch: %s\n", branchName)
			}
		}

		container, err := app.NewContainer(apiKey, cwd, runLlmModel, runDryRun, runCiMode, GetVerbose())
		if err != nil {
			return fmt.Errorf("container init failed: %w", err)
		}
		defer container.Close()

		container.ProjectPrimer.NoMap = runNoMap

		ctx := cmd.Context()

		fmt.Printf("🚀 Running task: %s\n", taskDescription)

		var primer string
		manualContextPath := filepath.Join(cwd, "codepicker_context.txt")

		if content, err := os.ReadFile(manualContextPath); err == nil {
			fmt.Println("🗺️  Using manual context file (codepicker_context.txt)...")
			primer = string(content)
		} else {
			fmt.Println("🗺️  Generating shallow project map (Depth 2) for planning...")
			primer = container.ProjectPrimer.GenerateShallow()
		}

		fmt.Println("🧠 Generating execution plan...")
		plan, err := container.Planner.CreatePlan(ctx, taskDescription, "", primer)
		if err != nil {
			return fmt.Errorf("planning failed: %w", err)
		}

		fmt.Printf("📜 Plan generated: %s (%d steps)\n", plan.ID, len(plan.Steps))

		if runCiMode {
			container.PlanExecutor.SetAutoConfirm(true)
		}

		err = container.PlanExecutor.Execute(ctx, plan)
		if err != nil {
			return fmt.Errorf("execution failed: %w", err)
		}

		fmt.Println("\n✅ Task Execution Completed.")
		for _, step := range plan.Steps {
			icon := "✅"
			if step.Status == task.StatusFailed {
				icon = "❌"
			}
			fmt.Printf("   %s Step %d: %s\n", icon, step.ID, step.Description)
		}

		if branchName != "" {
			fmt.Printf("\n📦 Session Summary:\n   Branch: %s\n   Run 'codepicker undo' to roll back edits.\n", branchName)
		}

		return nil
	},
}

func slugifyTask(s string) string {
	s = strings.ToLower(s)
	reg := regexp.MustCompile("[^a-z0-9]+")
	s = reg.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 25 {
		s = s[:25]
		s = strings.Trim(s, "-")
	}
	if s == "" {
		return "task"
	}
	return s
}

func init() {
	runCmd.Flags().BoolVar(&runDryRun, "dry-run", false, "Enable read-only mode")
	runCmd.Flags().BoolVar(&runCiMode, "ci", false, "Enable CI mode (skip confirmations)")
	runCmd.Flags().StringVar(&runLlmModel, "model", "", "LLM model to use")
	runCmd.Flags().BoolVarP(&runVerbose, "verbose", "v", false, "Enable verbose output")
	runCmd.Flags().BoolVar(&runNoMap, "no-map", false, "Disable the sparse repository map injection")
	runCmd.Flags().BoolVar(&runNoAutoCommit, "no-autocommit", false, "Disable auto-committing successful file edits")
	runCmd.Flags().BoolVarP(&runBranch, "branch", "b", false, "Create a new git branch for this session")
	rootCmd.AddCommand(runCmd)
}
