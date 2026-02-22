package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/david22573/codepicker/app"
	"github.com/david22573/codepicker/domain/task"
	"github.com/david22573/codepicker/infra/metrics"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var agentDryRun bool
var agentVerbose bool

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

		container, err := app.NewContainer(apiKey, cwd, "", agentDryRun, false, GetVerbose())
		if err != nil {
			fmt.Printf("❌ Container Init Failed: %v\n", err)
			os.Exit(1)
		}
		defer container.Close()

		metricsPort := 9090
		if container.Config != nil && container.Config.Server.MetricsPort != 0 {
			metricsPort = container.Config.Server.MetricsPort
		}

		metricsSrv := metrics.NewServer(metricsPort)
		metricsSrv.Start()

		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := metricsSrv.Shutdown(shutdownCtx); err != nil {
				fmt.Printf("⚠️ Metrics server shutdown error: %v\n", err)
			}
		}()

		fmt.Println(color.CyanString("🤖 CodePicker Agent initialized."))
		fmt.Println(color.HiBlackString("Type 'exit' or 'quit' to stop."))

		fmt.Print("🗺️  Loading project context... ")
		primer := container.ProjectPrimer.Generate()
		fmt.Println("Done.")

		basePrompt := container.PlanExecutor.GetSystemPrompt()
		cachedPrompt := fmt.Sprintf("%s\n\n### PROJECT CONTEXT (CACHED)\n%s", basePrompt, primer)

		container.PlanExecutor.UpdateSystemPrompt(cachedPrompt)

		reader := bufio.NewReader(os.Stdin)
		
		// 4.4 Signal Handling
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		for {
			fmt.Print(color.GreenString("\n> "))

			// Handle input reading in a way that respects context cancellation
			inputChan := make(chan string)
			go func() {
				in, _ := reader.ReadString('\n')
				inputChan <- in
			}()

			var input string
			select {
			case <-ctx.Done():
				fmt.Println("\nGracefully shutting down...")
				return
			case input = <-inputChan:
			}

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
						Instruction: input,
						Status:      task.StatusPending,
					},
				},
			}

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
	agentCmd.Flags().BoolVarP(&agentVerbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.AddCommand(agentCmd)
}