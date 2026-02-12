package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/david22573/codepicker/app"
	"github.com/david22573/codepicker/domain/task"
	"github.com/david22573/codepicker/infra/metrics" // <--- CRITICAL IMPORT
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var agentDryRun bool

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Start an interactive session with the CodePicker agent",
	Run: func(cmd *cobra.Command, args []string) {
		apiKey := os.Getenv("OPENROUTER_API_KEY")
		if apiKey == "" {
			fmt.Println("❌ Error: OPENROUTER_API_KEY is not set.")
			os.Exit(1)
		}

		cwd, _ := os.Getwd()

		// 1. Initialize Container
		container, err := app.NewContainer(apiKey, cwd, "", agentDryRun, false)
		if err != nil {
			fmt.Printf("❌ Container Init Failed: %v\n", err)
			os.Exit(1)
		}
		defer container.Close()

		// 2. Start Metrics & Health Server
		metricsPort := 9090
		if container.Config != nil && container.Config.Server.MetricsPort != 0 {
			metricsPort = container.Config.Server.MetricsPort
		}

		metricsSrv := metrics.NewServer(metricsPort)
		metricsSrv.Start()

		// Ensure graceful shutdown of metrics server
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := metricsSrv.Shutdown(ctx); err != nil {
				fmt.Printf("⚠️ Metrics server shutdown error: %v\n", err)
			}
		}()

		fmt.Println(color.CyanString("🤖 CodePicker Agent initialized."))
		fmt.Println(color.HiBlackString("Type 'exit' or 'quit' to stop."))

		// 3. Generate the Primer (Project Map)
		fmt.Print("🗺️  Loading project context... ")
		primer := container.ProjectPrimer.Generate()
		fmt.Println("Done.")

		reader := bufio.NewReader(os.Stdin)
		ctx := context.Background()

		// 4. Main Interactive Loop
		for {
			fmt.Print(color.GreenString("\n> "))
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)

			if input == "exit" || input == "quit" {
				break
			}
			if input == "clear" {
				fmt.Println("Context cleared.")
				continue
			}
			if input == "" {
				continue
			}

			// Prepend Primer to the input for immediate context
			enhancedInput := fmt.Sprintf("PROJECT CONTEXT:\n%s\n\nUSER REQUEST:\n%s", primer, input)
			fmt.Println(color.HiBlackString("Thinking..."))

			syntheticPlan := &task.Plan{
				ID:           "interactive-chat",
				OriginalTask: input,
				Reasoning:    "Interactive session user request",
				Status:       task.StatusPending,
				Steps: []task.Step{
					{
						ID:          1,
						Description: "Execute user request",
						Instruction: enhancedInput,
						Status:      task.StatusPending,
					},
				},
			}

			// Execute returns error; results are stored in the plan steps
			err := container.PlanExecutor.Execute(ctx, syntheticPlan)

			if err != nil {
				fmt.Printf("❌ Error: %v\n", err)
			} else {
				for _, step := range syntheticPlan.Steps {
					if step.Status == task.StatusFailed {
						fmt.Printf("❌ %s\n", step.Error)
					} else {
						fmt.Printf("✅ %s\n", step.Result)
					}
				}
			}
		}
	},
}

func init() {
	agentCmd.Flags().BoolVar(&agentDryRun, "dry-run", false, "Enable read-only mode")
	rootCmd.AddCommand(agentCmd)
}
