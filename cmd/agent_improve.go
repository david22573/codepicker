package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/david22573/codepicker/internal/app"
	"github.com/david22573/codepicker/internal/policy"
	"github.com/david22573/codepicker/internal/ui"
	"github.com/david22573/codepicker/pkg/openrouter"
	"github.com/spf13/cobra"
)

var improveLoop bool

var improveCmd = &cobra.Command{
	Use:     "improve",
	Short:   "Execute tasks from ARCHITECTURE_GOALS.md",
	Long:    `Reads the ARCHITECTURE_GOALS.md file, finds unchecked boxes, and executes them one by one. Use --loop to continuously run until all tasks are complete.`,
	Example: `  codepicker agent improve --loop`,
	RunE: func(cmd *cobra.Command, args []string) error {

		if ui.Standard == nil {
			ui.Standard = ui.NewConsoleUI()
		}

		goalsFile := filepath.Join(srcDir, "ARCHITECTURE_GOALS.md")

		// Infinite loop that breaks when tasks are done or on error (unless looping is forced)
		for {
			// 1. Freshly read the goals file every iteration
			content, err := os.ReadFile(goalsFile)
			if err != nil {
				return fmt.Errorf("could not find ARCHITECTURE_GOALS.md: %w\n(Run 'codepicker agent plan --architect' first)", err)
			}

			re := regexp.MustCompile(`(?m)^-\s\[\s\]\s(.*)$`)
			matches := re.FindSubmatch(content)

			if matches == nil || len(matches) < 2 {
				ui.Standard.Success("✨ All goals in ARCHITECTURE_GOALS.md are marked complete!")
				break
			}

			nextTask := string(matches[1])
			fullLine := string(matches[0])

			fmt.Println("\n" + strings.Repeat("=", 60))
			ui.Standard.Info("🎯 TARGET: \"%s\"", nextTask)
			fmt.Println(strings.Repeat("=", 60))

			// 2. Execute the task in an isolated scope to ensure proper cleanup (defer)
			err = func() error {
				// Initialize fresh context for THIS specific task
				agentCtx, err := app.NewAgentContext(cmd.Context(), app.ContextOptions{
					SrcDir:   srcDir,
					LogLevel: 1,
					Mode:     app.ModeInteractive,
					Policy:   policy.Batch, // Default to Batch for improvements to reduce friction
					Task:     nextTask,
				})
				if err != nil {
					return err
				}
				defer agentCtx.Close()

				// CONTEXT HYGIENE: Clear memory files from previous runs so the agent isn't confused
				if err := agentCtx.Store.ClearWorkingMemory(); err != nil {
					agentCtx.Logger.Warn("Failed to clear working memory: " + err.Error())
				}

				// Check Budget
				cost, _ := agentCtx.Engine.CostTracker.GetStats()
				if cost > agentCtx.Limits.DailyCostLimit {
					return fmt.Errorf("daily cost limit ($%.2f) reached", agentCtx.Limits.DailyCostLimit)
				}

				prompt := fmt.Sprintf("Your Goal: Implement this specific task from the improvement plan: '%s'. \n"+
					"1. Use 'search_code' to find relevant files.\n"+
					"2. Use 'read_file' to understand them.\n"+
					"3. Use 'write_shadow_file' to implement the fix.\n"+
					"4. Verify your work (run tests if applicable).\n"+
					"DO NOT update ARCHITECTURE_GOALS.md, I will handle that.", nextTask)

				printUpdate := func(msg openrouter.ChatMessage) {
					if msg.Role == "assistant" && msg.Content != nil {
						content := fmt.Sprintf("%v", msg.Content)
						if content != "" && !strings.Contains(content, "tool_calls") {
							fmt.Printf("\n🧠 Thought: %s\n", content)
						}
					}
				}

				ui.Standard.Info("🚀 Starting execution...")
				result, err := agentCtx.Engine.Run(agentCtx.Ctx, prompt, printUpdate)
				if err != nil {
					return err
				}

				fmt.Println("\n✅ Agent finished task.")
				fmt.Println("   " + result)
				return nil
			}()

			// 3. Handle Result
			if err != nil {
				ui.Standard.Error("\n🛑 Agent failed on task: %v", err)

				if !improveLoop {
					// In single mode, just fail
					return err
				}

				// In loop mode, ask user strategy
				choice, _, _ := ui.Standard.Select("How to proceed with this failure?", []string{
					"Retry (Run specific task again)",
					"Skip (Mark done & continue loop)",
					"Abort Loop (Exit)",
				})

				switch choice {
				case 0: // Retry
					fmt.Println("🔄 Retrying...")
					continue // Restart loop without marking done
				case 1: // Skip
					fmt.Println("⏭️  Skipping task...")
					// Fall through to mark as done
				case 2: // Abort
					return fmt.Errorf("loop aborted by user")
				}
			}

			// 4. Update the goals file
			// Re-read content just in case changed
			currentContent, _ := os.ReadFile(goalsFile)
			newContentStr := strings.Replace(string(currentContent), fullLine, "- [x] "+nextTask, 1)
			if err := os.WriteFile(goalsFile, []byte(newContentStr), 0644); err != nil {
				return fmt.Errorf("failed to update ARCHITECTURE_GOALS.md: %w", err)
			}

			ui.Standard.Success("📝 Marked task as complete in ARCHITECTURE_GOALS.md")
			ui.Standard.Info("👉 Run 'codepicker apply' to review changes so far.")

			if !improveLoop {
				break
			}

			// Small pause between tasks
			time.Sleep(1 * time.Second)
		}

		return nil
	},
}

func init() {
	agentCmd.AddCommand(improveCmd)
	improveCmd.Flags().BoolVarP(&improveLoop, "loop", "l", false, "Continuously execute tasks until all goals are complete")
}
