package cmd

import (
	"github.com/david22573/codepicker/internal/app"
	"github.com/david22573/codepicker/internal/policy"
	"github.com/david22573/codepicker/internal/server"
	"github.com/spf13/cobra"
)

var port string

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the codepicker agent daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		// 1. Boot the Runtime
		// Note: We use the strict 'Server' policy defined above
		agentCtx, err := app.NewAgentContext(cmd.Context(), app.ContextOptions{
			SrcDir:   srcDir,
			LogLevel: 1,
			Mode:     app.ModeServer,
			Policy:   policy.Server,
		})
		if err != nil {
			return err
		}
		defer agentCtx.Close()

		// 2. Initialize Server
		// The server now accepts the entire context, giving it access to
		// limits, db, engine, and config in one package.
		srv := server.New(port, agentCtx)

		// 3. Run
		// The context cancellation is handled by the server's signal trapping
		return srv.Start()
	},
}

func init() {
	// We still attach the command to root, but the logic is cleaner
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().StringVarP(&port, "port", "p", "22573", "Port to listen on")

	// Optional: Override src directory if different from CWD
	serveCmd.Flags().StringVarP(&srcDir, "src", "s", ".", "Source directory to allow agent access to")
}
