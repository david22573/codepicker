package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/david22573/codepicker/internal/agent"
	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/server"
	"github.com/david22573/codepicker/pkg/openrouter"
	"github.com/spf13/cobra"
)

var port string

var limits *config.Limits

func init() {
	limits = config.DefaultLimits()
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the codepicker agent daemon",
	RunE: func(cmd *cobra.Command, args []string) error {

		apiKey := os.Getenv("OPENROUTER_API_KEY")
		if apiKey == "" {
			return fmt.Errorf("OPENROUTER_API_KEY environment variable required")
		}

		absSrc, err := filepath.Abs(srcDir)
		if err != nil {
			return fmt.Errorf("failed to resolve source dir: %w", err)
		}

		client := openrouter.NewClient(apiKey)

		engine, err := agent.NewEngine(
			client,
			"xiaomi/mimo-v2-flash:free",
			absSrc,
			appLogger,
			limits,
		)
		if err != nil {
			return fmt.Errorf("failed to initialize agent engine: %w", err)
		}

		srv := server.New(port, engine, appLogger)
		return srv.Start()
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().StringVarP(&port, "port", "p", "22573", "Port to listen on")
}
