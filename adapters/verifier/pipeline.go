package verifier

import (
	"context"
	"fmt"

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
	Stage   string
	Logs    string
}

// Verify creates a sandbox, applies the block replacements, and runs standard Go checks.
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

	// 2. Run Checks
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
			return &VerifyResult{
				Success: false,
				Stage:   check.Name,
				Logs:    fmt.Sprintf("Error: %v\nOutput:\n%s", err, out),
			}, nil
		}
	}

	return &VerifyResult{Success: true}, nil
}

// ApplyToReal directly applies the verified SEARCH/REPLACE blocks to the actual project files.
func (p *Pipeline) ApplyToReal(patchDiff string) error {
	if err := fs.ApplySearchReplaceBlocks(p.ProjectRoot, patchDiff); err != nil {
		return fmt.Errorf("failed to apply verified blocks to real filesystem: %w", err)
	}
	return nil
}
