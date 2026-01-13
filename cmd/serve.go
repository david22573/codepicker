package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/david22573/codepicker/internal/agent"
	"github.com/david22573/codepicker/internal/server"
	"github.com/david22573/codepicker/pkg/openrouter"
	"github.com/spf13/cobra"
)

var port string

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the codepicker agent daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		// 1. Dependency Injection
		apiKey := os.Getenv("OPENROUTER_API_KEY")
		if apiKey == "" {
			return fmt.Errorf("OPENROUTER_API_KEY environment variable required")
		}

		absSrc, err := filepath.Abs(srcDir)
		if err != nil {
			return fmt.Errorf("failed to resolve source dir: %w", err)
		}

		client := openrouter.NewClient(apiKey)

		// 2. Initialize Core Logic (Brain)
		engine, err := agent.NewEngine(
			client,
			"xiaomi/mimo-v2-flash:free",
			absSrc,
			appLogger,
		)
		if err != nil {
			return fmt.Errorf("failed to initialize agent engine: %w", err)
		}

		// 3. Initialize Server (Interface)
		srv := server.New(port, engine, appLogger)

		appLogger.Info(fmt.Sprintf("📂 Context: %s", absSrc))
		appLogger.Info(fmt.Sprintf("🛡️  Shadow Workspace Active"))

		// 4. Start
		return srv.Start()
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().StringVarP(&port, "port", "p", "22573", "Port to listen on")
}
