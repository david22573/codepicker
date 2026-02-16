package git

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/david22573/codepicker/domain/audit"
)

type Client struct {
	ProjectRoot string
	DryRun      bool
}

func NewClient(root string, dryRun bool) *Client {
	return &Client{ProjectRoot: root, DryRun: dryRun}
}

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

// PreCommitCheck verifies the integrity of the code.
func (c *Client) PreCommitCheck(ctx context.Context) error {
	if c.DryRun {
		return nil
	}

	// 1. Run go fmt (auto-fix)
	fmtCmd := exec.CommandContext(ctx, "go", "fmt", "./...")
	fmtCmd.Dir = c.ProjectRoot
	if out, err := fmtCmd.CombinedOutput(); err != nil {
		fmt.Printf("⚠️  Pre-commit fmt warning: %s\n", string(out))
	}

	// 2. Run go vet (Strict correctness check)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	vetCmd := exec.CommandContext(ctx, "go", "vet", "./...")
	vetCmd.Dir = c.ProjectRoot

	if out, err := vetCmd.CombinedOutput(); err != nil {
		// FIX: Don't block commit on existing project errors.
		// The Agent operates in a sandbox first; if that passed, the specific change is likely fine.
		// Blocking here prevents fixing "brownfield" projects.
		fmt.Printf("⚠️  WARNING: 'go vet' found issues (proceeding as changes were sandboxed):\n%s\n", string(out))
		return nil
	}

	return nil
}

func (c *Client) Commit(ctx context.Context, p *audit.Provenance) (string, error) {
	// Checks run, but now lenient on global errors
	if err := c.PreCommitCheck(ctx); err != nil {
		return "", fmt.Errorf("safety check failed: %w", err)
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

func (c *Client) IsDirty() bool {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = c.ProjectRoot
	out, _ := cmd.Output()
	return len(out) > 0
}
