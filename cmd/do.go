package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/internal/agent"
	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/pkg/openrouter"
	"github.com/spf13/cobra"
)

var doCmd = &cobra.Command{
	Use:   "do [task]",
	Short: "Execute an autonomous agent task (enables tools & recovery)",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		task := strings.Join(args, " ")

		apiKey, err := validateAPIKey()
		if err != nil {
			return err
		}

		absSrc, err := filepath.Abs(srcDir)
		if err != nil {
			return err
		}

		client := openrouter.NewClient(apiKey)
		limits := config.DefaultLimits()

		// Initialize the Agent Engine (which contains our Phase 1 Recovery logic)
		eng, err := agent.NewEngine(client, "xiaomi/mimo-v2-flash:free", absSrc, appLogger, limits)
		if err != nil {
			return err
		}

		// Simple CLI approval mechanism
		eng.ApprovalCallback = func(c, r string) bool {
			fmt.Printf("\n⚠️  Agent wants to run: %s\n   Reason: %s\n   Allow? [Y/n]: ", c, r)
			var resp string
			fmt.Scanln(&resp)
			return resp == "" || resp == "y" || resp == "Y"
		}

		appLogger.Info("🤖 Agent starting task: " + task)

		// Define a callback to print "thoughts" and partial responses to the console
		printUpdate := func(msg openrouter.ChatMessage) {
			if msg.Role == "assistant" && msg.Content != nil {
				content := fmt.Sprintf("%v", msg.Content)
				if content != "" {
					fmt.Printf("\n🤖 Thought: %s\n", content)
				}
			}
			if msg.Role == "tool" {
				// We don't print full tool output here to keep it clean,
				// relying on the Engine logger for that.
			}
		}

		result, err := eng.Run(cmd.Context(), task, printUpdate)
		if err != nil {
			return err
		}

		fmt.Printf("\n✅ Task Completed. Final Result:\n%s\n", result)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(doCmd)
}
