package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/mcp"
	"github.com/david22573/codepicker/internal/ui"
	"github.com/spf13/cobra"
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Validate configuration and extensions",
	Long:  `Validates the .codepicker.yml file, tests connections to configured MCP servers, and verifies custom tool definitions. Use this to debug your environment.`,
	Run: func(cmd *cobra.Command, args []string) {
		if ui.Standard == nil {
			ui.Standard = ui.NewConsoleUI()
		}

		fmt.Println("🔍 Running System Check...\n")
		allPassed := true

		// 1. Check API Key
		if key := os.Getenv("OPENROUTER_API_KEY"); key == "" {
			ui.Standard.Error("❌ OPENROUTER_API_KEY is missing")
			allPassed = false
		} else {
			ui.Standard.Success("✅ OPENROUTER_API_KEY found")
		}

		// 2. Load Config
		cfg, err := config.GetOrLoadConfig("")
		if err != nil {
			ui.Standard.Error("❌ Config Load Error: %v", err)
			allPassed = false
			return // Cannot proceed without config
		} else {
			ui.Standard.Success("✅ Config loaded: .codepicker.yml")
		}

		// 3. Validate Custom Tools
		if len(cfg.CustomTools) > 0 {
			fmt.Println("\n🛠️  Verifying Custom Tools:")
			for _, t := range cfg.CustomTools {
				if t.Name == "" || t.Command == "" {
					ui.Standard.Error("  ❌ Invalid tool definition (missing name or command)")
					allPassed = false
				} else {
					ui.Standard.Info("  • %s: OK", t.Name)
				}
			}
		}

		// 4. Test MCP Connections
		if len(cfg.MCPServers) > 0 {
			fmt.Println("\n🔌 Testing MCP Servers:")
			log := logger.NewStandardLogger(1)
			mgr := mcp.NewManager(log)

			// We manually start them to test connection
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			mgr.StartServers(ctx, cfg.MCPServers)

			// Wait a brief moment for handshake
			time.Sleep(1 * time.Second)

			tools, err := mgr.GetTools(ctx)
			if err != nil {
				ui.Standard.Error("  ❌ Failed to list MCP tools: %v", err)
				allPassed = false
			} else {
				connectedCount := len(mgr.Clients)
				if connectedCount == len(cfg.MCPServers) {
					ui.Standard.Success("  ✅ All %d servers connected", connectedCount)
				} else {
					ui.Standard.Warn("  ⚠️  Only %d/%d servers connected", connectedCount, len(cfg.MCPServers))
					allPassed = false
				}
				ui.Standard.Info("  • Discovered %d remote capabilities", len(tools))
			}
			mgr.CloseAll()
		}

		fmt.Println("")
		if allPassed {
			ui.Standard.Success("✨ System check passed. Codepicker is ready.")
		} else {
			ui.Standard.Error("🛑 System check failed. Please fix the issues above.")
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(checkCmd)
}
