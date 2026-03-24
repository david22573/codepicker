package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/david22573/codepicker/app"
	"github.com/david22573/codepicker/infra/git"
	"github.com/spf13/cobra"
)

var fixDryRun bool
var fixLlmModel string
var fixVerbose bool
var fixBranch bool
var fixResume string

var fixCmd = &cobra.Command{
	Use:   "fix [file path]",
	Short: "Apply automated fixes to a file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiKey := getAPIKeyOrExit()
		targetFile := args[0]
		cwd, _ := os.Getwd()

		gitClient := git.NewClient(cwd, fixDryRun)
		var branchName string
		if fixBranch {
			base := filepath.Base(targetFile)
			base = strings.ReplaceAll(base, ".", "-")
			branchName = fmt.Sprintf("cp/%d-fix-%s", time.Now().Unix(), base)
			if err := gitClient.CreateBranch(cmd.Context(), branchName); err != nil {
				fmt.Printf("⚠️  Failed to create session branch: %v\n", err)
			} else {
				fmt.Printf("🌿 Switched to new session branch: %s\n", branchName)
			}
		}

		container, err := app.NewContainer(apiKey, cwd, fixLlmModel, fixDryRun, false, GetVerbose())
		if err != nil {
			return fmt.Errorf("container init failed: %w", err)
		}
		defer container.Close()

		ctx := cmd.Context()
		task := "Fix bugs, handle unhandled errors, and improve code quality."

		var resumeBlock string
		if fixResume != "" {
			prevSession, err := container.Repository.GetSession(ctx, fixResume)
			if err != nil {
				return fmt.Errorf("failed to load session %s: %w", fixResume, err)
			}
			fmt.Printf("⏪ Resuming fix session: %s\n", prevSession.ID)

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("\n### RESUMED FIX SESSION (%s)\n", prevSession.ID))

			if len(prevSession.EditsMade) > 0 {
				sb.WriteString("Edits Already Made in Previous Run:\n")
				for _, edit := range prevSession.EditsMade {
					sb.WriteString(fmt.Sprintf("- %s\n", edit))
				}
			}

			sb.WriteString("\nPrevious Session Context:\n")
			for _, m := range prevSession.Messages {
				sb.WriteString(fmt.Sprintf("[%s]: %s\n", m.Role, m.Content))
			}
			resumeBlock = sb.String()
		}

		fmt.Printf("🔧 Analyzing %s...\n", targetFile)

		// 1. Primer (Context)
		primer := container.ProjectPrimer.Generate()
		primer += resumeBlock

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

		if branchName != "" {
			fmt.Printf("\n📦 Session Summary:\n   Branch: %s\n   Run 'codepicker undo' to roll back edits.\n", branchName)
		}

		return nil
	},
}

func init() {
	fixCmd.Flags().BoolVar(&fixDryRun, "dry-run", false, "Enable read-only mode")
	fixCmd.Flags().StringVar(&fixLlmModel, "model", "", "LLM model to use")
	fixCmd.Flags().BoolVarP(&fixVerbose, "verbose", "v", false, "Enable verbose output")
	fixCmd.Flags().BoolVarP(&fixBranch, "branch", "b", false, "Create a new git branch for this fix session")
	fixCmd.Flags().StringVar(&fixResume, "resume", "", "Resume a previous session by ID")
	rootCmd.AddCommand(fixCmd)
}
