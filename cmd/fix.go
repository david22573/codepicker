package cmd

import (
	"fmt"
	"os"

	"github.com/david22573/codepicker/app"
	"github.com/spf13/cobra"
)

var fixDryRun bool
var fixLlmModel string
var fixVerbose bool

var fixCmd = &cobra.Command{
	Use:   "fix [file path]",
	Short: "Apply automated fixes to a file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiKey := getAPIKeyOrExit()
		targetFile := args[0]
		cwd, _ := os.Getwd()

		container, err := app.NewContainer(apiKey, cwd, fixLlmModel, fixDryRun, false, GetVerbose())
		if err != nil {
			return fmt.Errorf("container init failed: %w", err)
		}
		defer container.Close()

		ctx := cmd.Context()
		task := "Fix bugs, handle unhandled errors, and improve code quality."
		fmt.Printf("🔧 Analyzing %s...\n", targetFile)

		// 1. Primer (Context)
		primer := container.ProjectPrimer.Generate()

		// 2. Phase 1: Analysis (Read-Only)
		analysis, err := container.TwoPassEngine.RunAnalysis(ctx, task, targetFile, primer)
		if err != nil {
			return fmt.Errorf("analysis failed: %w", err)
		}
		fmt.Printf("💡 Diagnosis: %s\n", analysis.Markdown)

		// 3. Phase 2: Generation (Engineer)
		fmt.Println("🏗️  Generating patch...")
		patch, err := container.TwoPassEngine.GeneratePatch(ctx, task, analysis)
		if err != nil {
			return fmt.Errorf("patch generation failed: %w", err)
		}

		// 4. Verification (Sandbox)
		fmt.Println("🧪 Verifying patch in sandbox...")
		verifyRes, err := container.Verifier.Verify(ctx, patch.Diff)
		if err != nil {
			return fmt.Errorf("verification error: %w", err)
		}

		if !verifyRes.Success {
			return fmt.Errorf("verification failed at stage '%s':\n%s", verifyRes.Stage, verifyRes.Logs)
		}

		// 5. Apply
		fmt.Println("✅ Patch verified. Applying to real filesystem...")
		if err := container.Verifier.ApplyToReal(patch.Diff); err != nil {
			return fmt.Errorf("apply failed: %w", err)
		}

		fmt.Println("✨ Fix applied successfully!")
		return nil
	},
}

func init() {
	fixCmd.Flags().BoolVar(&fixDryRun, "dry-run", false, "Enable read-only mode")
	fixCmd.Flags().StringVar(&fixLlmModel, "model", "", "LLM model to use")
	fixCmd.Flags().BoolVarP(&fixVerbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.AddCommand(fixCmd)
}
