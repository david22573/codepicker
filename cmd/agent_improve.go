package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/david22573/codepicker/internal/app"
	"github.com/david22573/codepicker/internal/policy"
	"github.com/david22573/codepicker/internal/ui"
	"github.com/david22573/codepicker/pkg/openrouter"
	"github.com/spf13/cobra"
)

var improveCmd = &cobra.Command{
	Use:     "improve",
	Short:   "Execute the next task from ARCHITECTURE_GOALS.md",
	Long:    `Reads the ARCHITECTURE_GOALS.md file, finds the first unchecked box, and instructs the agent to implement it. Automatically updates the file upon completion.`,
	Example: `  codepicker agent improve`,
	RunE: func(cmd *cobra.Command, args []string) error {

		if ui.Standard == nil {
			ui.Standard = ui.NewConsoleUI()
		}

		goalsFile := filepath.Join(srcDir, "ARCHITECTURE_GOALS.md")
		content, err := os.ReadFile(goalsFile)
		if err != nil {
			return fmt.Errorf("could not find ARCHITECTURE_GOALS.md: %w\n(Run 'codepicker agent plan --architect' first)", err)
		}

		re := regexp.MustCompile(`(?m)^-\s\[\s\]\s(.*)$`)
		matches := re.FindSubmatch(content)

		if matches == nil || len(matches) < 2 {
			fmt.Println("✨ All goals in ARCHITECTURE_GOALS.md are marked complete!")
			return nil
		}

		nextTask := string(matches[1])
		fullLine := string(matches[0])

		fmt.Printf("\n🎯 \033[1mNext Goal Identified:\033[0m \"%s\"\n", nextTask)

		agentCtx, err := app.NewAgentContext(cmd.Context(), app.ContextOptions{
			SrcDir:   srcDir,
			LogLevel: 1,
			Mode:     app.ModeInteractive,
			Policy:   policy.Interactive,
			Task:     nextTask,
		})
		if err != nil {
			return err
		}
		defer agentCtx.Close()

		prompt := fmt.Sprintf("Your Goal: Implement this specific task from the improvement plan: '%s'. \n"+
			"Verify your work with tests if possible. \n"+
			"When finished, you DO NOT need to update ARCHITECTURE_GOALS.md, I will handle that.", nextTask)

		printUpdate := func(msg openrouter.ChatMessage) {
			if msg.Role == "assistant" && msg.Content != nil {
				content := fmt.Sprintf("%v", msg.Content)
				if content != "" && !strings.Contains(content, "tool_calls") {
					fmt.Printf("\n🧠 Thought: %s\n", content)
				}
			}
		}

		fmt.Println("🚀 Starting execution...")

		// RETRY LOOP: This prevents the 30-turn crash
		var result string
		for {
			result, err = agentCtx.Engine.Run(agentCtx.Ctx, prompt, printUpdate)
			if err == nil {
				break
			}

			ui.Standard.Error("\n🛑 Agent stopped: %v", err)

			// If it's the context limit or turn limit, we can usually continue or retry
			choice, _, _ := ui.Standard.Select("How do you want to proceed?", []string{
				"Retry (Resume/Try Again)",
				"Skip (Mark as Done anyway)",
				"Abort (Exit)",
			})

			if choice == 0 {
				fmt.Println("🔄 Retrying...")
				// Optional: Append a "continue" instruction if we wanted to be smarter,
				// but a raw retry often works if it was a network/random failure.
				continue
			} else if choice == 1 {
				fmt.Println("⏭️  Skipping task (assuming it was completed manually or is minor).")
				result = "Skipped by user."
				break
			} else {
				return fmt.Errorf("agent failed to complete task: %w", err)
			}
		}

		fmt.Println("\n✅ Task execution finished.")
		fmt.Println("   " + result)

		newContent := strings.Replace(string(content), fullLine, "- [x] "+nextTask, 1)
		if err := os.WriteFile(goalsFile, []byte(newContent), 0644); err != nil {
			return fmt.Errorf("failed to update ARCHITECTURE_GOALS.md: %w", err)
		}

		fmt.Println("\n📝 Updated ARCHITECTURE_GOALS.md marked as complete.")
		fmt.Println("👉 Check '.codepicker/shadow' to review and apply the code changes.")
		return nil
	},
}

func init() {
	agentCmd.AddCommand(improveCmd)
}
