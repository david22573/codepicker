package verifier

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/domain/task"
	"github.com/david22573/codepicker/infra/fs"
)

// Pipeline manages the verification steps for a proposed patch.
type Pipeline struct {
	ProjectRoot string
	Commands    []string
	FailClosed  bool
}

func NewPipeline(root string) *Pipeline {
	return &Pipeline{
		ProjectRoot: root,
		Commands:    []string{},
		FailClosed:  true,
	}
}

// VerifyResult holds the outcome of the verification process.
type VerifyResult struct {
	Success bool
	Stage   string
	Logs    string
	Report  *task.CheckReport
}

// Verify creates a sandbox, applies the block replacements, and runs checks.
func (p *Pipeline) Verify(ctx context.Context, patchDiff string) (*VerifyResult, error) {
	fmt.Println("🧪 [VERIFY] Creating Sandbox Environment...")

	sandbox, err := fs.NewSandbox(p.ProjectRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to init sandbox: %w", err)
	}
	defer sandbox.Cleanup()

	// 1. Apply Blocks
	fmt.Println("🧪 [VERIFY] Applying Blocks to Sandbox...")
	if err := sandbox.ApplyPatch([]byte(patchDiff)); err != nil {
		return &VerifyResult{
			Success: false,
			Stage:   "apply blocks",
			Logs:    err.Error(),
		}, nil
	}

	// 2. Determine Checks
	var checks []string
	if len(p.Commands) > 0 {
		checks = p.Commands
	} else {
		// Auto-detect defaults based on file presence in ProjectRoot
		if fileExists(filepath.Join(p.ProjectRoot, "go.mod")) {
			checks = []string{"go test ./...", "go vet ./...", "go build ./..."}
		} else if fileExists(filepath.Join(p.ProjectRoot, "pnpm-lock.yaml")) {
			checks = []string{"pnpm test", "pnpm build"}
		} else if fileExists(filepath.Join(p.ProjectRoot, "package.json")) {
			checks = []string{"npm test", "npm run build"}
		} else if fileExists(filepath.Join(p.ProjectRoot, "requirements.txt")) || fileExists(filepath.Join(p.ProjectRoot, "pyproject.toml")) || fileExists(filepath.Join(p.ProjectRoot, "Pipfile")) {
			checks = []string{"pytest", "python -m compileall ."}
		} else {
			// fallback
			if p.FailClosed {
				checks = nil // block if fail_closed and no commands found
			} else {
				checks = []string{"go test ./...", "go vet ./...", "go build ./..."}
			}
		}
	}

	report := &task.CheckReport{
		Status: "pass",
		Checks: []task.CheckResult{},
	}

	if len(checks) == 0 && p.FailClosed {
		return &VerifyResult{
			Success: false,
			Stage:   "command selection",
			Logs:    "fail_closed is true but no verifier commands detected",
			Report:  report,
		}, nil
	}

	overallSuccess := true
	for _, check := range checks {
		trimmed := strings.TrimSpace(check)
		if trimmed == "" {
			continue
		}

		fmt.Printf("🧪 [VERIFY] Running '%s'...\n", trimmed)
		res := sandbox.RunCommandCheck(ctx, trimmed, trimmed)
		report.Checks = append(report.Checks, res)

		if res.Status != task.CheckPass {
			overallSuccess = false
			report.Status = "fail"
		}
	}

	return &VerifyResult{
		Success: overallSuccess,
		Report:  report,
		Logs:    summarizeReport(report),
	}, nil
}

func summarizeReport(report *task.CheckReport) string {
	var sb strings.Builder
	for _, res := range report.Checks {
		status := "PASS"
		if res.Status != task.CheckPass {
			status = "FAIL"
		}
		sb.WriteString(fmt.Sprintf("[%s] %s\n", status, res.Name))
		if res.Error != "" {
			sb.WriteString(fmt.Sprintf("  Error: %s\n", res.Error))
		}
		if res.Stderr != "" {
			sb.WriteString(fmt.Sprintf("  Stderr:\n%s\n", res.Stderr))
		}
	}
	return sb.String()
}

// ApplyToReal directly applies the verified SEARCH/REPLACE blocks to the actual project files.
func (p *Pipeline) ApplyToReal(patchDiff string) error {
	if err := fs.ApplySearchReplaceBlocks(p.ProjectRoot, patchDiff); err != nil {
		return fmt.Errorf("failed to apply verified blocks to real filesystem: %w", err)
	}
	return nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func splitCommand(cmd string) (string, []string) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return "", nil
	}
	return parts[0], parts[1:]
}
