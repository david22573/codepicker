package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/internal/agent"
	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/constants"
	"github.com/david22573/codepicker/internal/database"
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

		// Initialize Database
		store, err := database.New(".codepicker")
		if err != nil {
			return fmt.Errorf("failed to init database: %w", err)
		}
		defer store.Close()

		client := openrouter.NewClient(apiKey)
		limits := config.DefaultLimits()

		eng, err := agent.NewEngine(client, constants.DefaultModel, absSrc, appLogger, limits, store)
		if err != nil {
			return err
		}

		eng.ApprovalCallback = func(c, r string) bool {
			fmt.Printf("\n⚠️  Agent wants to run: %s\n   Reason: %s\n   Allow? [Y/n]: ", c, r)
			var resp string
			fmt.Scanln(&resp)
			return resp == "" || resp == "y" || resp == "Y"
		}

		appLogger.Info("🤖 Agent starting task: " + task)

		printUpdate := func(msg openrouter.ChatMessage) {
			if msg.Role == "assistant" && msg.Content != nil {
				content := fmt.Sprintf("%v", msg.Content)
				if content != "" {
					fmt.Printf("\n🤖 Thought: %s\n", content)
				}
			}
			if msg.Role == "tool" {
				// Optional: print tool outputs
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
