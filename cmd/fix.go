package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/david22573/codepicker/app"
	"github.com/spf13/cobra"
)

var fixTargetFile string

var fixCmd = &cobra.Command{
	Use:   "fix [task]",
	Short: "Auto-fix a bug using the Two-Pass (Analyst/Engineer) workflow",
	Long: `The fix command uses a specialized two-stage process:
1. Analyst: Reads the code and diagnoses the root cause.
2. Engineer: Writes a patch to fix the issue.

Requires a task description and a target file.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		taskInput := args[0]
		apiKey := os.Getenv("OPENROUTER_API_KEY")
		if apiKey == "" {
			fmt.Println("❌ Error: OPENROUTER_API_KEY environment variable is required.")
			os.Exit(1)
		}

		cwd, _ := os.Getwd()

		// FIX: Update NewContainer call signature
		container, err := app.NewContainer(apiKey, cwd, "", dryRunFlag, ciFlag)
		if err != nil {
			fmt.Printf("❌ Container Init Failed: %v\n", err)
			os.Exit(1)
		}

		ctx := context.Background()

		if fixTargetFile == "" {
			fmt.Println("❌ Error: You must specify a target file with --file or -f")
			return
		}

		// --- Phase 1: The Analyst ---
		fmt.Printf("🧐 [ANALYST] Diagnosing issue in %s...\n", fixTargetFile)

		// The Analyst reads the specific file to understand the context
		analysis, err := container.TwoPassEngine.RunAnalysis(ctx, taskInput, fixTargetFile)
		if err != nil {
			fmt.Printf("❌ Analysis Failed: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("\n📄 [REPORT] Findings:")
		fmt.Println(analysis.Markdown)

		// --- Phase 2: The Engineer ---
		fmt.Println("\n👷 [ENGINEER] Generating patch...")

		// The Engineer generates a Git patch based on the Analyst's report
		patch, err := container.TwoPassEngine.GeneratePatch(ctx, taskInput, analysis)
		if err != nil {
			fmt.Printf("❌ Patch Generation Failed: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("\n📝 [PATCH] Generated Diff:")
		fmt.Println(patch.Diff)

		// --- Phase 3: Verification (Optional) ---
		if !dryRunFlag {
			fmt.Println("\n🧪 [VERIFIER] Verifying patch in sandbox...")
			result, err := container.Verifier.Verify(ctx, patch.Diff)
			if err != nil {
				fmt.Printf("⚠️  Verification Error: %v\n", err)
			} else if !result.Success {
				fmt.Printf("❌ Verification Failed during %s:\n%s\n", result.Stage, result.Logs)

				// Optional: Trigger Self-Correction (RefinePatch) here if desired
				// patch, err = container.TwoPassEngine.RefinePatch(ctx, taskInput, analysis, patch.Diff, result.Logs)
			} else {
				fmt.Println("✅ Verification Passed! Applying to real codebase...")
				if err := container.Verifier.ApplyToReal("patch.diff"); err != nil {
					// Note: You might need to save patch.diff to disk first depending on implementation
					fmt.Printf("⚠️  Could not auto-apply. Save the diff manually.\n")
				}
			}
		}
	},
}

func init() {
	fixCmd.Flags().StringVarP(&fixTargetFile, "file", "f", "", "Target file to analyze and fix")
	rootCmd.AddCommand(fixCmd)
}
