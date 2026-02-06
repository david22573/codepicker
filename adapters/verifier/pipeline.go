package verifier

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/david22573/codepicker/infra/fs"
)

// Pipeline manages the verification steps for a proposed patch.
type Pipeline struct {
	ProjectRoot string
}

func NewPipeline(root string) *Pipeline {
	return &Pipeline{ProjectRoot: root}
}

// VerifyResult holds the outcome of the verification process.
type VerifyResult struct {
	Success bool
	Stage   string // which stage failed (e.g., "go test")
	Logs    string // output from the failed command
}

// Verify creates a sandbox, applies the patch, and runs the standard Go checks.
func (p *Pipeline) Verify(ctx context.Context, patchDiff string) (*VerifyResult, error) {
	fmt.Println("🧪 [VERIFY] Creating Sandbox Environment...")

	sandbox, err := fs.NewSandbox(p.ProjectRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to init sandbox: %w", err)
	}
	defer sandbox.Cleanup()

	// 1. Apply Patch
	fmt.Println("🧪 [VERIFY] Applying Patch to Sandbox...")
	if err := sandbox.ApplyPatch([]byte(patchDiff)); err != nil {
		return &VerifyResult{
			Success: false,
			Stage:   "git apply",
			Logs:    err.Error(),
		}, nil
	}

	// 2. Run Checks
	// We define the standard pipeline: Vet -> Test -> Build
	checks := []struct {
		Name string
		Args []string
	}{
		{"go vet", []string{"vet", "./..."}},
		{"go test", []string{"test", "./..."}},
		{"go build", []string{"build", "./..."}},
	}

	for _, check := range checks {
		fmt.Printf("🧪 [VERIFY] Running '%s'...\n", check.Name)
		out, err := sandbox.RunGoCommand(ctx, check.Args...)
		if err != nil {
			// Verification Failed
			return &VerifyResult{
				Success: false,
				Stage:   check.Name,
				Logs:    fmt.Sprintf("Error: %v\nOutput:\n%s", err, out),
			}, nil
		}
	}

	return &VerifyResult{Success: true}, nil
}

// ApplyToReal effectively "merges" the verified patch to the real codebase.
func (p *Pipeline) ApplyToReal(patchPath string) error {
	cmd := exec.Command("git", "apply", patchPath)
	cmd.Dir = p.ProjectRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("apply failed: %s", string(out))
	}
	return nil
}
