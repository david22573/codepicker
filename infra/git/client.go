package git

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/david22573/codepicker/domain/audit"
)

// Client wraps git command line operations.
type Client struct {
	ProjectRoot string
	DryRun      bool
}

func NewClient(root string, dryRun bool) *Client {
	return &Client{
		ProjectRoot: root,
		DryRun:      dryRun,
	}
}

// StageAll runs 'git add .' to stage all changes.
func (c *Client) StageAll() error {
	if c.DryRun {
		fmt.Println("🔒 [DRY-RUN] Skipping 'git add .'")
		return nil
	}

	cmd := exec.Command("git", "add", ".")
	cmd.Dir = c.ProjectRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add failed: %s", string(out))
	}
	return nil
}

// PreCommitCheck verifies the integrity of the code before committing.
// OPTIMIZATION: Runs go vet and go fmt to ensure quality.
func (c *Client) PreCommitCheck(ctx context.Context) error {
	if c.DryRun {
		return nil
	}

	// 1. Run go fmt (auto-fix)
	fmtCmd := exec.CommandContext(ctx, "go", "fmt", "./...")
	fmtCmd.Dir = c.ProjectRoot
	if out, err := fmtCmd.CombinedOutput(); err != nil {
		// We don't fail on fmt error, but we log it.
		// Often go fmt fails if syntax is invalid, which 'go vet' will catch next.
		fmt.Printf("⚠️  Pre-commit fmt warning: %s\n", string(out))
	}

	// 2. Run go vet (Strict correctness check)
	// We set a timeout to prevent hanging on large codebases
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	vetCmd := exec.CommandContext(ctx, "go", "vet", "./...")
	vetCmd.Dir = c.ProjectRoot

	if out, err := vetCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("pre-commit check failed (go vet):\n%s", string(out))
	}

	return nil
}

// Commit creates a new commit with the provenance message.
func (c *Client) Commit(ctx context.Context, p *audit.Provenance) (string, error) {
	// OPTIMIZATION: Run safety checks first
	if err := c.PreCommitCheck(ctx); err != nil {
		return "", fmt.Errorf("safety check failed, commit aborted: %w", err)
	}

	msg := p.FormatCommitMessage()

	if c.DryRun {
		fmt.Printf("🔒 [DRY-RUN] Would commit with message:\n%s\n", msg)
		return "dry-run-hash", nil
	}

	cmd := exec.CommandContext(ctx, "git", "commit", "-m", msg)
	cmd.Dir = c.ProjectRoot

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git commit failed: %s", string(out))
	}

	return strings.TrimSpace(string(out)), nil
}

// IsDirty checks if there are uncommitted changes.
func (c *Client) IsDirty() bool {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = c.ProjectRoot
	out, _ := cmd.Output()
	return len(out) > 0
}
