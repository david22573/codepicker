package cmd

import (
	"context"
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
	Run: func(cmd *cobra.Command, args []string) {
		apiKey := os.Getenv("OPENROUTER_API_KEY")
		if apiKey == "" {
			fmt.Println("❌ Error: OPENROUTER_API_KEY is not set.")
			os.Exit(1)
		}

		targetFile := args[0]
		cwd, _ := os.Getwd()

		// Initialize Container with verbose flag
		container, err := app.NewContainer(apiKey, cwd, fixLlmModel, fixDryRun, false, GetVerbose())
		if err != nil {
			fmt.Printf("❌ Container Init Failed: %v\n", err)
			os.Exit(1)
		}
		defer container.Close()

		ctx := context.Background()
		task := "Fix bugs, handle unhandled errors, and improve code quality."
		fmt.Printf("🔧 Analyzing %s...\n", targetFile)

		// 1. Primer (Context)
		primer := container.ProjectPrimer.Generate()

		// 2. Phase 1: Analysis (Read-Only)
		analysis, err := container.TwoPassEngine.RunAnalysis(ctx, task, targetFile, primer)
		if err != nil {
			fmt.Printf("❌ Analysis failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("💡 Diagnosis: %s\n", analysis.Markdown)

		// 3. Phase 2: Generation (Engineer)
		fmt.Println("🏗️  Generating patch...")
		patch, err := container.TwoPassEngine.GeneratePatch(ctx, task, analysis)
		if err != nil {
			fmt.Printf("❌ Patch generation failed: %v\n", err)
			os.Exit(1)
		}

		// 4. Verification (Sandbox)
		fmt.Println("🧪 Verifying patch in sandbox...")
		verifyRes, err := container.Verifier.Verify(ctx, patch.Diff)
		if err != nil {
			fmt.Printf("❌ Verification error: %v\n", err)
			os.Exit(1)
		}

		if !verifyRes.Success {
			fmt.Printf("❌ Verification failed at stage '%s':\n%s\n", verifyRes.Stage, verifyRes.Logs)
			os.Exit(1)
		}

		// 5. Apply
		fmt.Println("✅ Patch verified. Applying to real filesystem...")
		if err := container.Verifier.ApplyToReal(patch.Diff); err != nil {
			fmt.Printf("❌ Apply failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✨ Fix applied successfully!")
	},
}

func init() {
	fixCmd.Flags().BoolVar(&fixDryRun, "dry-run", false, "Enable read-only mode")
	fixCmd.Flags().StringVar(&fixLlmModel, "model", "", "LLM model to use")
	fixCmd.Flags().BoolVarP(&fixVerbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.AddCommand(fixCmd)
}
