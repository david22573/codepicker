package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/david22573/codepicker/internal/agent"
	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/constants"
	"github.com/david22573/codepicker/internal/database"
	"github.com/david22573/codepicker/pkg/openrouter"
	"github.com/spf13/cobra"
)

var improveCmd = &cobra.Command{
	Use:   "improve",
	Short: "Execute the next task from ARCHITECTURE_GOALS.md",
	Long:  `Reads the ARCHITECTURE_GOALS.md file, finds the first unchecked box, and instructs the agent to implement it.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// 1. Read the Goals File
		goalsFile := filepath.Join(srcDir, "ARCHITECTURE_GOALS.md")
		content, err := os.ReadFile(goalsFile)
		if err != nil {
			return fmt.Errorf("could not find ARCHITECTURE_GOALS.md: %w\n(Run 'codepicker audit' first)", err)
		}

		// 2. Find the next task
		// Regex matches "- [ ] Some task text"
		re := regexp.MustCompile(`(?m)^-\s\[\s\]\s(.*)$`)
		matches := re.FindSubmatch(content)

		if matches == nil || len(matches) < 2 {
			fmt.Println("✨ All goals in ARCHITECTURE_GOALS.md are marked complete!")
			return nil
		}

		nextTask := string(matches[1])
		fullLine := string(matches[0])

		fmt.Printf("\n🎯 Next Goal Identified: \"%s\"\n", nextTask)
		fmt.Printf("   Starting agent run...\n\n")

		// 3. Initialize Agent
		apiKey, err := validateAPIKey()
		if err != nil {
			return err
		}
		absSrc, _ := filepath.Abs(srcDir)
		store, err := database.New(".codepicker")
		if err != nil {
			return err
		}
		defer store.Close()

		client := openrouter.NewClient(apiKey)
		eng, err := agent.NewEngine(client, constants.DefaultModel, absSrc, appLogger, config.DefaultLimits(), store)
		if err != nil {
			return err
		}

		// 4. Run the Agent
		eng.ApprovalCallback = func(c, r string) bool {
			fmt.Printf("\n⚠️  Agent wants to run: %s\n   Reason: %s\n   Allow? [Y/n]: ", c, r)
			var resp string
			fmt.Scanln(&resp)
			return resp == "" || resp == "y" || resp == "Y"
		}

		prompt := fmt.Sprintf("Your Goal: Implement this specific task from the improvement plan: '%s'. \n"+
			"Verify your work with tests if possible. \n"+
			"When finished, you DO NOT need to update ARCHITECTURE_GOALS.md, I will handle that.", nextTask)

		printUpdate := func(msg openrouter.ChatMessage) {
			if msg.Role == "assistant" && msg.Content != nil {
				fmt.Printf("🤖 %v\n", msg.Content)
			}
		}

		result, err := eng.Run(cmd.Context(), prompt, printUpdate)
		if err != nil {
			return fmt.Errorf("agent failed to complete task: %w", err)
		}

		// 5. Update the Markdown file if successful
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
	rootCmd.AddCommand(improveCmd)
}
