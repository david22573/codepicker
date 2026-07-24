package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/david22573/codepicker/app"
	"github.com/spf13/cobra"
)

var (
	improvePick     int
	improvePlanOnly bool
	improveDryRun   bool
	improveApply    bool
	improveBranch   bool
	improveVerbose  bool
	improveLlmModel string
)

var improveCmd = &cobra.Command{
	Use:   "improve",
	Short: "Automatically suggest and apply codebase improvements",
	Long: `Scans the repository to identify structural, performance, or styling areas of improvement.
Under the hood, suggests 3 high-value tasks, lets you pick one, and delegates to the unified run execution engine.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Non-LLM part initially to fetch suggestions
		cwd, _ := os.Getwd()

		// Get an API key (we only need it if we actually proceed to RunTask, but SuggestImprovements also needs the LLM stack)
		apiKey := getAPIKeyOrExit("improve")

		container, err := app.NewContainer(apiKey, cwd, improveLlmModel, true, false, GetVerbose())
		if err != nil {
			return fmt.Errorf("container init failed: %w", err)
		}

		fmt.Println("🗺️  Building project map...")
		primer := container.ProjectPrimer.Generate()

		fmt.Println("📡 [SCOUT] Searching for potential improvements...")
		tasks, err := container.Auditor.SuggestImprovements(cmd.Context(), primer)
		if err != nil {
			container.Close()
			return fmt.Errorf("audit failed: %w", err)
		}
		container.Close() // Close early since we'll initialize a new container inside RunTask

		if len(tasks) == 0 {
			fmt.Println("✅ No immediate improvements suggested. Your code is looking sharp!")
			return nil
		}

		fmt.Printf("\n✨ Found %d suggested improvements:\n", len(tasks))
		for i, t := range tasks {
			fmt.Printf("%d. %s\n", i+1, t)
		}

		// Non-interactive choice if --pick is specified
		var selectedIndex = improvePick - 1
		if improvePick == 0 {
			// Interactive choice
			fmt.Print("\nSelect a task number to apply [1-3] or press Enter to quit: ")
			reader := bufio.NewReader(os.Stdin)
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)
			if input == "" {
				fmt.Println("Quit.")
				return nil
			}

			choice, err := strconv.Atoi(input)
			if err != nil || choice < 1 || choice > len(tasks) {
				return fmt.Errorf("invalid choice: %s", input)
			}
			selectedIndex = choice - 1
		} else if selectedIndex < 0 || selectedIndex >= len(tasks) {
			return fmt.Errorf("invalid pick value: %d (must be between 1 and %d)", improvePick, len(tasks))
		}

		selectedTask := tasks[selectedIndex]
		fmt.Printf("\n🚀 Proceeding with selected task: \"%s\"\n\n", selectedTask)

		opts := RunOptions{
			TaskDescription: selectedTask,
			PlanOnly:        improvePlanOnly,
			DryRun:          improveDryRun,
			Apply:           improveApply,
			Branch:          improveBranch,
			LlmModel:        improveLlmModel,
		}

		return RunTask(cmd.Context(), opts)
	},
}

func init() {
	improveCmd.Flags().IntVar(&improvePick, "pick", 0, "Index of suggested improvement to apply directly (non-interactive)")
	improveCmd.Flags().BoolVar(&improvePlanOnly, "plan-only", false, "Generate plan, print details, but make zero changes")
	improveCmd.Flags().BoolVar(&improveDryRun, "dry-run", false, "Execute against shadow/sandbox only; make zero real changes")
	improveCmd.Flags().BoolVar(&improveApply, "apply", false, "Apply verified changes to the real filesystem")
	improveCmd.Flags().BoolVarP(&improveBranch, "branch", "b", false, "Create a new git branch for this improvement session")
	improveCmd.Flags().BoolVarP(&improveVerbose, "verbose", "v", false, "Enable verbose output")
	improveCmd.Flags().StringVar(&improveLlmModel, "model", "", "LLM model to use")
	rootCmd.AddCommand(improveCmd)
}
