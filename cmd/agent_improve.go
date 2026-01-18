package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/david22573/codepicker/internal/app"
	"github.com/david22573/codepicker/internal/policy"
	"github.com/david22573/codepicker/pkg/openrouter"
	"github.com/spf13/cobra"
)

var improveCmd = &cobra.Command{
	Use:     "improve",
	Short:   "Execute the next task from ARCHITECTURE_GOALS.md",
	Long:    `Reads the ARCHITECTURE_GOALS.md file, finds the first unchecked box, and instructs the agent to implement it. Automatically updates the file upon completion.`,
	Example: `  codepicker agent improve`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// 1. Locate and Parse Goals
		// We do this *before* spinning up the agent to fail fast if the file is missing.
		goalsFile := filepath.Join(srcDir, "ARCHITECTURE_GOALS.md") // srcDir is global from root.go
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

		// 2. Initialize Agent Context
		// using ModeInteractive ensures we get the "Allow? [y/N]" prompts for shell commands
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

		// 3. Construct Prompt
		prompt := fmt.Sprintf("Your Goal: Implement this specific task from the improvement plan: '%s'. \n"+
			"Verify your work with tests if possible. \n"+
			"When finished, you DO NOT need to update ARCHITECTURE_GOALS.md, I will handle that.", nextTask)

		// 4. Output Handler
		printUpdate := func(msg openrouter.ChatMessage) {
			if msg.Role == "assistant" && msg.Content != nil {
				content := fmt.Sprintf("%v", msg.Content)
				if content != "" && !strings.Contains(content, "tool_calls") {
					fmt.Printf("\n🧠 Thought: %s\n", content)
				}
			}
		}

		// 5. Run Execution
		fmt.Println("🚀 Starting execution...")
		result, err := agentCtx.Engine.Run(agentCtx.Ctx, prompt, printUpdate)
		if err != nil {
			return fmt.Errorf("agent failed to complete task: %w", err)
		}

		fmt.Println("\n✅ Task execution finished.")
		fmt.Println("   " + result)

		// 6. Update the Goals File
		// We assume success if the agent didn't error out.
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
	// Register 'improve' under the 'agent' namespace
	agentCmd.AddCommand(improveCmd)
}
