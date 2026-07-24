package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/david22573/codepicker/app"
	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/task"
	"github.com/david22573/codepicker/infra/git"
)

type RunOptions struct {
	TaskDescription string
	TargetFile      string // for fix command
	DryRun          bool
	PlanOnly        bool
	Apply           bool
	Branch          bool
	CiMode          bool
	LlmModel        string
	NoMap           bool
	NoAutoCommit    bool
	ResumeSessionID string
	Force           bool
}

func RunTask(ctx context.Context, opts RunOptions) (err error) {
	var origStdout *os.File
	if GetJSON() {
		origStdout = os.Stdout
		os.Stdout = os.Stderr
		defer func() {
			os.Stdout = origStdout
			if err != nil {
				runJSON := map[string]interface{}{
					"status": "fail",
					"error":  err.Error(),
				}
				jsonData, _ := json.Marshal(runJSON)
				fmt.Fprintln(origStdout, string(jsonData))
			}
		}()
	}

	// 1. API Key check
	apiKey := getAPIKeyOrExit("run")
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	// 2. Default mode safety enforcement
	if !opts.Apply && !opts.DryRun && !opts.PlanOnly {
		opts.PlanOnly = true
		fmt.Println("ℹ️  No mode specified. Defaulting to safe `--plan-only` mode.")
	}

	// 3. Generate run ID and setup artifact directory
	runID := fmt.Sprintf("run-%s", time.Now().Format("20060102-150405"))
	runDir := filepath.Join(cwd, ".codepicker", "runs", runID)
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return fmt.Errorf("failed to create run directory: %w", err)
	}

	// Save task input
	taskMDPath := filepath.Join(runDir, "task.md")
	taskMDContent := fmt.Sprintf("# CodePicker Task Description\n\n%s\n", opts.TaskDescription)
	_ = os.WriteFile(taskMDPath, []byte(taskMDContent), 0644)

	// 4. Branch creation if --branch
	gitClient := git.NewClient(cwd, opts.PlanOnly || opts.DryRun)
	var branchName string
	if opts.Branch && opts.TaskDescription != "" {
		slug := slugifyTask(opts.TaskDescription)
		timestamp := time.Now().Format("20060102-150405")
		branchName = fmt.Sprintf("codepicker/%s-%s", slug, timestamp)

		if !opts.PlanOnly && !opts.DryRun {
			if err := gitClient.CreateBranch(ctx, branchName); err != nil {
				fmt.Printf("⚠️  Failed to create session branch: %v\n", err)
			} else {
				fmt.Printf("🌿 Switched to new session branch: %s\n", branchName)
			}
		} else {
			fmt.Printf("🌿 [DRY-RUN] Would switch to new session branch: %s\n", branchName)
		}
	}

	if opts.NoAutoCommit {
		os.Setenv("CODEPICKER_NO_AUTOCOMMIT", "1")
	}

	// 5. Container initialization
	// If PlanOnly or DryRun, container runs in dry-run mode
	containerDryRun := opts.PlanOnly || opts.DryRun
	container, err := app.NewContainer(apiKey, cwd, opts.LlmModel, containerDryRun, opts.CiMode, GetVerbose())
	if err != nil {
		return fmt.Errorf("container init failed: %w", err)
	}
	defer container.Close()

	container.ProjectPrimer.NoMap = opts.NoMap

	// 6. Build Context / Primer
	var resumeBlock string
	sessionID := opts.ResumeSessionID
	if sessionID == "" {
		sessionID = fmt.Sprintf("sess_%d", time.Now().Unix())
	} else {
		prevSession, err := container.Repository.GetSession(ctx, sessionID)
		if err == nil {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("\n### RESUMED SESSION (%s)\n", prevSession.ID))
			for _, m := range prevSession.Messages {
				sb.WriteString(fmt.Sprintf("[%s]: %s\n", m.Role, m.Content))
			}
			resumeBlock = sb.String()
		}
	}

	// Prime context
	var primer string
	manualContextPath := filepath.Join(cwd, "codepicker_context.txt")
	if content, err := os.ReadFile(manualContextPath); err == nil {
		fmt.Println("🗺️  Using manual context file (codepicker_context.txt)...")
		primer = string(content)
	} else {
		fmt.Println("🗺️  Generating shallow project map (Depth 2) for planning...")
		primer = container.ProjectPrimer.GenerateShallow()
	}
	primer += resumeBlock

	// Retrieve dynamic past learnings
	if container.EmbedClient != nil {
		vectors, _, err := container.EmbedClient.CreateEmbeddings(ctx, []string{opts.TaskDescription})
		if err == nil && len(vectors) > 0 {
			learnings, err := container.Repository.SearchLearnings(ctx, vectors[0], 3)
			if err == nil && len(learnings) > 0 {
				fmt.Printf("💡 Recalled %d relevant past learning(s).\n", len(learnings))
				primer += "\n\n### PAST LEARNINGS & NOTES\n"
				for _, l := range learnings {
					primer += fmt.Sprintf("- (From task: %s) %s\n", l.Task, l.Note)
				}
			}
		}
	}

	// Save packed context to artifact
	packedContextPath := filepath.Join(runDir, "packed_context.md")
	_ = os.WriteFile(packedContextPath, []byte(primer), 0644)

	// Save run record to SQLite
	currentSession := &agent.Session{
		ID:        sessionID,
		Task:      opts.TaskDescription,
		CreatedAt: time.Now(),
		Outcome:   "running",
	}
	_ = container.Repository.SaveSession(ctx, currentSession)

	var verifyResLogs string
	var patchDiff string
	var runOutcome = "completed"

	// 7. Core Execution Path
	if opts.TargetFile != "" {
		// FIX command execution flow using TwoPassEngine
		fmt.Printf("🔧 Analyzing %s...\n", opts.TargetFile)
		analysis, err := container.TwoPassEngine.RunAnalysis(ctx, opts.TaskDescription, opts.TargetFile, primer)
		if err != nil {
			_ = os.WriteFile(filepath.Join(runDir, "summary.md"), []byte(fmt.Sprintf("# Run Summary\n\nStatus: FAIL\nError: Analysis failed: %v\n", err)), 0644)
			return fmt.Errorf("analysis failed: %w", err)
		}
		fmt.Printf("💡 Diagnosis: %s\n", analysis.Markdown)

		// Save analysis.md
		_ = os.WriteFile(filepath.Join(runDir, "analysis.md"), []byte(analysis.Markdown), 0644)

		fmt.Println("🏗️  Generating patch...")
		patch, err := container.TwoPassEngine.GeneratePatch(ctx, opts.TaskDescription, analysis)
		if err != nil {
			_ = os.WriteFile(filepath.Join(runDir, "summary.md"), []byte(fmt.Sprintf("# Run Summary\n\nStatus: FAIL\nError: Patch generation failed: %v\n", err)), 0644)
			return fmt.Errorf("patch generation failed: %w", err)
		}

		patchDiff = patch.Diff
		_ = os.WriteFile(filepath.Join(runDir, "patch.diff"), []byte(patchDiff), 0644)

		// Sandbox Verification
		fmt.Println("🧪 Verifying patch in sandbox...")
		verifyRes, err := container.Verifier.Verify(ctx, patch.Diff)

		if verifyRes != nil && verifyRes.Report != nil {
			reportJSON, _ := json.MarshalIndent(verifyRes.Report, "", "  ")
			_ = os.WriteFile(filepath.Join(runDir, "verifier.json"), reportJSON, 0644)

			var verifierMD strings.Builder
			verifierMD.WriteString("# Verifier Report\n\n")
			verifierMD.WriteString(fmt.Sprintf("Status: %s\n\n", strings.ToUpper(verifyRes.Report.Status)))
			for _, c := range verifyRes.Report.Checks {
				status := "✅ PASS"
				if c.Status != task.CheckPass {
					status = "❌ FAIL"
				}
				verifierMD.WriteString(fmt.Sprintf("### %s %s\n", status, c.Name))
				verifierMD.WriteString(fmt.Sprintf("- Command: `%s`\n", c.Command))
				verifierMD.WriteString(fmt.Sprintf("- Duration: %dms\n", c.DurationMS))
				if c.Error != "" {
					verifierMD.WriteString(fmt.Sprintf("- Error: %s\n", c.Error))
				}
				if c.Stdout != "" {
					verifierMD.WriteString(fmt.Sprintf("\n#### Stdout\n```\n%s\n```\n", c.Stdout))
				}
				if c.Stderr != "" {
					verifierMD.WriteString(fmt.Sprintf("\n#### Stderr\n```\n%s\n```\n", c.Stderr))
				}
			}
			_ = os.WriteFile(filepath.Join(runDir, "verifier.md"), []byte(verifierMD.String()), 0644)
		}

		if err != nil {
			verifyResLogs = err.Error()
			_ = os.WriteFile(filepath.Join(runDir, "verifier.log"), []byte(verifyResLogs), 0644)
			return fmt.Errorf("verification error: %w", err)
		}

		verifyResLogs = verifyRes.Logs
		_ = os.WriteFile(filepath.Join(runDir, "verifier.log"), []byte(verifyResLogs), 0644)

		if !verifyRes.Success && !opts.Force {
			runOutcome = "failed"
			_ = os.WriteFile(filepath.Join(runDir, "summary.md"), []byte(fmt.Sprintf("# Run Summary\n\nStatus: FAIL\nError: Verification failed at stage '%s'\n", verifyRes.Stage)), 0644)
			fmt.Printf("\n❌ Verifier FAILED at stage '%s'.\n", verifyRes.Stage)
			fmt.Println(verifyRes.Logs)
			fmt.Println("\n⚠️  Apply blocked. Use --force to override.")
			return fmt.Errorf("verification failed at stage '%s'", verifyRes.Stage)
		}

		// Apply if requested
		if opts.Apply && !opts.DryRun && !opts.PlanOnly {
			fmt.Println("✅ Patch verified. Applying to real filesystem...")

			// Show changed file summary before applying (transaction backups)
			tx, txErr := container.WorkspaceManager.BeginTransaction()
			if txErr != nil {
				return fmt.Errorf("failed to start transaction: %w", txErr)
			}
			defer tx.Rollback()

			// Backup files
			_ = tx.BackupFile(opts.TargetFile)

			if err := container.Verifier.ApplyToReal(patch.Diff); err != nil {
				runOutcome = "failed"
				return fmt.Errorf("apply failed: %w", err)
			}

			_ = tx.Commit()
			fmt.Println("✨ Fix applied successfully!")
		} else {
			fmt.Println("ℹ️  Safe mode: zero real file changes made.")
		}

	} else {
		// General RUN execution flow using PlanExecutor
		fmt.Println("🧠 Generating execution plan...")
		plan, err := container.Planner.CreatePlan(ctx, opts.TaskDescription, "", primer)
		if err != nil {
			_ = os.WriteFile(filepath.Join(runDir, "summary.md"), []byte(fmt.Sprintf("# Run Summary\n\nStatus: FAIL\nError: Planning failed: %v\n", err)), 0644)
			return fmt.Errorf("planning failed: %w", err)
		}

		fmt.Printf("📜 Plan generated: %s (%d steps)\n", plan.ID, len(plan.Steps))

		// Save plan.json and plan.md
		planJSON, _ := json.MarshalIndent(plan, "", "  ")
		_ = os.WriteFile(filepath.Join(runDir, "plan.json"), planJSON, 0644)

		var planMD strings.Builder
		planMD.WriteString(fmt.Sprintf("# CodePicker Execution Plan: %s\n\n", plan.ID))
		planMD.WriteString(fmt.Sprintf("## Strategy\n%s\n\n", plan.Reasoning))
		planMD.WriteString("## Steps\n")
		for _, s := range plan.Steps {
			planMD.WriteString(fmt.Sprintf("- Step %d: %s (Targets: %v)\n", s.ID, s.Description, s.Files))
		}
		_ = os.WriteFile(filepath.Join(runDir, "plan.md"), []byte(planMD.String()), 0644)

		if opts.PlanOnly {
			// Print target files and return
			fmt.Println("\n📋 Target Files planned to change:")
			for _, step := range plan.Steps {
				for _, f := range step.Files {
					fmt.Printf("   - %s\n", f)
				}
			}
			fmt.Printf("\n💾 Saved plan: %s\n", plan.ID)
			fmt.Println("ℹ️  Safe `--plan-only` mode: zero file changes made.")

			// Save summary.md for PlanOnly
			_ = os.WriteFile(filepath.Join(runDir, "summary.md"), []byte(fmt.Sprintf("# CodePicker Run Summary\n\n## Status\nPASS (Plan Only)\n\n## Plan ID\n%s\n\n## Task\n%s\n", plan.ID, opts.TaskDescription)), 0644)
			return nil
		}

		if opts.CiMode {
			container.PlanExecutor.SetAutoConfirm(true)
		}

		// Execute against shadow
		fmt.Println("🧪 Executing plan in shadow/sandbox...")
		err = container.PlanExecutor.Execute(ctx, plan)
		if err != nil {
			runOutcome = "failed"
			currentSession.Outcome = "failed"
			_ = container.Repository.SaveSession(ctx, currentSession)

			// Generate shadow diff if possible even on failure
			shadowFiles, _ := container.ShadowManager.ListShadowFiles()
			if len(shadowFiles) > 0 {
				var diffBuilder strings.Builder
				for _, sf := range shadowFiles {
					oldContent, _ := os.ReadFile(filepath.Join(cwd, sf))
					newContent, _ := container.ShadowManager.Read(sf)
					diffBuilder.WriteString(computeDiff(sf, string(oldContent), string(newContent)))
				}
				patchDiff = diffBuilder.String()
				_ = os.WriteFile(filepath.Join(runDir, "patch.diff"), []byte(patchDiff), 0644)
			}

			_ = os.WriteFile(filepath.Join(runDir, "summary.md"), []byte(fmt.Sprintf("# Run Summary\n\nStatus: FAIL\nError: Execution failed: %v\n", err)), 0644)
			return fmt.Errorf("execution failed: %w", err)
		}

		currentSession.Outcome = "completed"
		for _, step := range plan.Steps {
			if step.Status == task.StatusCompleted && len(step.Files) > 0 {
				currentSession.EditsMade = append(currentSession.EditsMade, step.Files...)
			}
		}
		_ = container.Repository.SaveSession(ctx, currentSession)

		// Generate diff of shadow writes
		shadowFiles, _ := container.ShadowManager.ListShadowFiles()
		if len(shadowFiles) > 0 {
			var diffBuilder strings.Builder
			for _, sf := range shadowFiles {
				oldContent, _ := os.ReadFile(filepath.Join(cwd, sf))
				newContent, _ := container.ShadowManager.Read(sf)
				diffBuilder.WriteString(computeDiff(sf, string(oldContent), string(newContent)))
			}
			patchDiff = diffBuilder.String()
			_ = os.WriteFile(filepath.Join(runDir, "patch.diff"), []byte(patchDiff), 0644)
		}

		// Sandbox Verification
		fmt.Println("🧪 Running sandbox verification checks...")
		verifyRes, err := container.Verifier.Verify(ctx, patchDiff)

		if verifyRes != nil && verifyRes.Report != nil {
			reportJSON, _ := json.MarshalIndent(verifyRes.Report, "", "  ")
			_ = os.WriteFile(filepath.Join(runDir, "verifier.json"), reportJSON, 0644)

			var verifierMD strings.Builder
			verifierMD.WriteString("# Verifier Report\n\n")
			verifierMD.WriteString(fmt.Sprintf("Status: %s\n\n", strings.ToUpper(verifyRes.Report.Status)))
			for _, c := range verifyRes.Report.Checks {
				status := "✅ PASS"
				if c.Status != task.CheckPass {
					status = "❌ FAIL"
				}
				verifierMD.WriteString(fmt.Sprintf("### %s %s\n", status, c.Name))
				verifierMD.WriteString(fmt.Sprintf("- Command: `%s`\n", c.Command))
				verifierMD.WriteString(fmt.Sprintf("- Duration: %dms\n", c.DurationMS))
				if c.Error != "" {
					verifierMD.WriteString(fmt.Sprintf("- Error: %s\n", c.Error))
				}
				if c.Stdout != "" {
					verifierMD.WriteString(fmt.Sprintf("\n#### Stdout\n```\n%s\n```\n", c.Stdout))
				}
				if c.Stderr != "" {
					verifierMD.WriteString(fmt.Sprintf("\n#### Stderr\n```\n%s\n```\n", c.Stderr))
				}
			}
			_ = os.WriteFile(filepath.Join(runDir, "verifier.md"), []byte(verifierMD.String()), 0644)
		}

		if err != nil {
			verifyResLogs = err.Error()
			_ = os.WriteFile(filepath.Join(runDir, "verifier.log"), []byte(verifyResLogs), 0644)
		} else {
			verifyResLogs = verifyRes.Logs
			_ = os.WriteFile(filepath.Join(runDir, "verifier.log"), []byte(verifyResLogs), 0644)
		}

		// Phase 3: Block apply on verifier fail
		if verifyRes != nil && !verifyRes.Success && !opts.Force {
			runOutcome = "failed"
			_ = os.WriteFile(filepath.Join(runDir, "summary.md"), []byte(fmt.Sprintf("# Run Summary\n\nStatus: FAIL\nError: Verification failed: %s\n", verifyRes.Logs)), 0644)

			fmt.Printf("\n❌ Verifier FAILED at stage '%s'.\n", verifyRes.Stage)
			fmt.Println(verifyRes.Logs)
			fmt.Println("\n⚠️  Apply blocked to prevent breaking the repository.")
			fmt.Println("💡 Use --force to apply anyway.")

			if opts.Apply {
				return fmt.Errorf("verification failed: %s", verifyRes.Stage)
			}
		}

		// Apply changes if `--apply` is specified
		if opts.Apply && !opts.DryRun {
			// Show changed files and ask proceed
			fmt.Println("\n📦 Preview of Files to change:")
			for _, sf := range shadowFiles {
				fmt.Printf("   - [MODIFY] %s\n", sf)
			}

			proceed := opts.CiMode
			if !proceed {
				fmt.Print("\nProceed with apply? [y/N]: ")
				reader := bufio.NewReader(os.Stdin)
				input, _ := reader.ReadString('\n')
				input = strings.TrimSpace(strings.ToLower(input))
				proceed = (input == "y" || input == "yes")
			}

			if proceed {
				fmt.Println("🚀 Applying shadow changes to real filesystem...")
				tx, txErr := container.WorkspaceManager.BeginTransaction()
				if txErr != nil {
					return fmt.Errorf("failed to start transaction: %w", txErr)
				}
				defer tx.Rollback()

				// Backup files
				for _, sf := range shadowFiles {
					_ = tx.BackupFile(sf)
				}

				// Copy shadow files to backups directory
				backupRunDir := filepath.Join(runDir, "backups")
				_ = os.MkdirAll(backupRunDir, 0755)
				for _, sf := range shadowFiles {
					oldContent, _ := os.ReadFile(filepath.Join(cwd, sf))
					_ = os.WriteFile(filepath.Join(backupRunDir, filepath.Base(sf)), oldContent, 0644)
				}

				successCount := 0
				for _, file := range shadowFiles {
					err := container.ShadowManager.Commit(file)
					if err != nil {
						fmt.Printf("❌ Failed to apply '%s': %v\n", file, err)
					} else {
						fmt.Printf("✅ Applied: %s\n", file)
						successCount++
					}
				}

				if successCount == len(shadowFiles) {
					_ = tx.Commit()
					fmt.Println("🎉 All changes applied successfully.")
				} else {
					runOutcome = "failed"
					return fmt.Errorf("applied %d/%d files with errors", successCount, len(shadowFiles))
				}
			} else {
				fmt.Println("❌ Apply aborted by user. Changes kept in shadow directory (.codepicker/shadow).")
			}
		} else {
			fmt.Println("ℹ️  Safe `--dry-run` mode: zero real file changes made. Changes preserved in shadow directory.")
		}
	}

	// 8. Cost Snapshot & cost.json
	metricsSnap := container.CostTracker.GetMetrics()
	costMap := map[string]interface{}{
		"total_tokens":      metricsSnap.TotalTokens,
		"prompt_tokens":     metricsSnap.PromptTokens,
		"completion_tokens": metricsSnap.CompletionTokens,
		"total_cost":        metricsSnap.TotalCost,
		"request_count":     metricsSnap.RequestCount,
	}
	costJSON, _ := json.MarshalIndent(costMap, "", "  ")
	_ = os.WriteFile(filepath.Join(runDir, "cost.json"), costJSON, 0644)

	// 9. summary.md
	var summaryMD strings.Builder
	summaryMD.WriteString("# CodePicker Run Summary\n\n")
	summaryMD.WriteString(fmt.Sprintf("## Status\n%s\n\n", strings.ToUpper(runOutcome)))
	summaryMD.WriteString(fmt.Sprintf("## Task\n%s\n\n", opts.TaskDescription))
	summaryMD.WriteString("## Files Changed\n")
	shadowFiles, _ := container.ShadowManager.ListShadowFiles()
	for _, sf := range shadowFiles {
		summaryMD.WriteString(fmt.Sprintf("- %s\n", sf))
	}
	if len(shadowFiles) == 0 {
		summaryMD.WriteString("(none)\n")
	}
	summaryMD.WriteString("\n## Verification\n")
	if verifyResLogs != "" {
		summaryMD.WriteString(fmt.Sprintf("```\n%s\n```\n", verifyResLogs))
	} else {
		summaryMD.WriteString("Skipped or not available.\n")
	}
	summaryMD.WriteString(fmt.Sprintf("\n## Cost\nTokens: %d, Cost: $%.5f\n\n", metricsSnap.TotalTokens, metricsSnap.TotalCost))
	summaryMD.WriteString("## Next Command\n")
	summaryMD.WriteString("To undo this run, execute:\n```bash\ncodepicker undo --last\n```\n")

	_ = os.WriteFile(filepath.Join(runDir, "summary.md"), []byte(summaryMD.String()), 0644)

	if GetJSON() {
		os.Stdout = origStdout

		status := "pass"
		if runOutcome == "failed" {
			status = "fail"
		}

		checksJson := []map[string]interface{}{}
		if verifyResLogs != "" {
			verifierStatus := "pass"
			if runOutcome == "failed" {
				verifierStatus = "fail"
			}
			checksJson = append(checksJson, map[string]interface{}{
				"name":   "verifier",
				"status": verifierStatus,
			})
		}

		runJSON := map[string]interface{}{
			"status":        status,
			"run_id":        runID,
			"artifacts_dir": runDir,
			"checks":        checksJson,
		}

		jsonData, _ := json.MarshalIndent(runJSON, "", "  ")
		fmt.Println(string(jsonData))
	} else {
		// Output summary to console
		fmt.Printf("\n✨ Run Completed. Artifacts saved under: %s\n", runDir)
	}
	return nil
}

// Pure Go helper to compute unified diff format
func computeDiff(relPath string, oldText, newText string) string {
	oldLines := strings.Split(oldText, "\n")
	newLines := strings.Split(newText, "\n")
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("diff --git a/%s b/%s\n", relPath, relPath))
	sb.WriteString(fmt.Sprintf("--- a/%s\n", relPath))
	sb.WriteString(fmt.Sprintf("+++ b/%s\n", relPath))

	if len(oldLines) == 1 && oldLines[0] == "" {
		sb.WriteString(fmt.Sprintf("@@ -0,0 +1,%d @@\n", len(newLines)))
		for _, line := range newLines {
			sb.WriteString("+" + line + "\n")
		}
		return sb.String()
	}
	if len(newLines) == 1 && newLines[0] == "" {
		sb.WriteString(fmt.Sprintf("@@ -1,%d +0,0 @@\n", len(oldLines)))
		for _, line := range oldLines {
			sb.WriteString("-" + line + "\n")
		}
		return sb.String()
	}

	sb.WriteString("@@ -1,1 +1,1 @@\n")
	i, j := 0, 0
	for i < len(oldLines) && j < len(newLines) {
		if oldLines[i] == newLines[j] {
			sb.WriteString(" " + oldLines[i] + "\n")
			i++
			j++
		} else {
			matchIdxOld, matchIdxNew := -1, -1
			for x := i; x < len(oldLines); x++ {
				for y := j; y < len(newLines); y++ {
					if oldLines[x] == newLines[y] {
						matchIdxOld = x
						matchIdxNew = y
						break
					}
				}
				if matchIdxOld != -1 {
					break
				}
			}
			if matchIdxOld != -1 {
				for x := i; x < matchIdxOld; x++ {
					sb.WriteString("-" + oldLines[x] + "\n")
				}
				for y := j; y < matchIdxNew; y++ {
					sb.WriteString("+" + newLines[y] + "\n")
				}
				i = matchIdxOld
				j = matchIdxNew
			} else {
				for x := i; x < len(oldLines); x++ {
					sb.WriteString("-" + oldLines[x] + "\n")
				}
				for y := j; y < len(newLines); y++ {
					sb.WriteString("+" + newLines[y] + "\n")
				}
				break
			}
		}
	}
	return sb.String()
}

func slugifyTask(s string) string {
	s = strings.ToLower(s)
	reg := regexp.MustCompile("[^a-z0-9]+")
	s = reg.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 25 {
		s = s[:25]
		s = strings.Trim(s, "-")
	}
	if s == "" {
		return "task"
	}
	return s
}
