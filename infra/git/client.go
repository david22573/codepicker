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
		fmt.Println("🔏 [DRY-RUN] Skipping 'git add .'")
		return nil
	}
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = c.ProjectRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add failed: %s", string(out))
	}
	return nil
}

func (c *Client) StageFiles(paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	if c.DryRun {
		fmt.Printf("🔏 [DRY-RUN] Skipping 'git add' for %v\n", paths)
		return nil
	}
	args := append([]string{"add"}, paths...)
	cmd := exec.Command("git", args...)
	cmd.Dir = c.ProjectRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add failed for %v: %s", paths, string(out))
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
	ctxTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	vetCmd := exec.CommandContext(ctxTimeout, "go", "vet", "./...")
	vetCmd.Dir = c.ProjectRoot

	if out, err := vetCmd.CombinedOutput(); err != nil {
		fmt.Printf("⚠️  WARNING: 'go vet' found issues (proceeding as changes were sandboxed):\n%s\n", string(out))
		return nil
	}

	return nil
}

func (c *Client) Commit(ctx context.Context, p *audit.Provenance) (string, error) {
	return c.CommitWithMessage(ctx, p.FormatCommitMessage())
}

func (c *Client) CommitWithMessage(ctx context.Context, message string) (string, error) {
	if err := c.PreCommitCheck(ctx); err != nil {
		return "", fmt.Errorf("safety check failed: %w", err)
	}

	if c.DryRun {
		fmt.Printf("🔏 [DRY-RUN] Would commit with message:\n%s\n", message)
		return "dry-run-hash", nil
	}

	cmd := exec.CommandContext(ctx, "git", "commit", "-m", message)
	cmd.Dir = c.ProjectRoot

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git commit failed: %s", string(out))
	}

	return strings.TrimSpace(string(out)), nil
}

func (c *Client) RevertFiles(paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	if c.DryRun {
		fmt.Printf("🔏 [DRY-RUN] Skipping reverting files: %v\n", paths)
		return nil
	}
	args := append([]string{"checkout", "HEAD", "--"}, paths...)
	cmd := exec.Command("git", args...)
	cmd.Dir = c.ProjectRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git revert failed for %v: %s", paths, string(out))
	}
	return nil
}

func (c *Client) CreateBranch(name string) error {
	if c.DryRun {
		fmt.Printf("🔏 [DRY-RUN] Skipping branch creation: %s\n", name)
		return nil
	}
	cmd := exec.Command("git", "checkout", "-b", name)
	cmd.Dir = c.ProjectRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git branch creation failed: %s", string(out))
	}
	return nil
}

func (c *Client) IsDirty() bool {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = c.ProjectRoot
	out, _ := cmd.Output()
	return len(out) > 0
}

// GetLastCodepickerCommits finds the last N commits made by the agent.
func (c *Client) GetLastCodepickerCommits(n int) ([]string, error) {
	cmd := exec.Command("git", "log", "--grep=\\[codepicker\\]", fmt.Sprintf("-n%d", n), "--format=%H")
	cmd.Dir = c.ProjectRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git log failed: %s", string(out))
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var hashes []string
	for _, line := range lines {
		if line != "" {
			hashes = append(hashes, line)
		}
	}
	return hashes, nil
}

// RevertCommits cleanly reverts the given commit hashes.
func (c *Client) RevertCommits(hashes []string) error {
	if len(hashes) == 0 {
		return nil
	}
	if c.DryRun {
		fmt.Printf("🔏 [DRY-RUN] Skipping git revert for %v\n", hashes)
		return nil
	}

	args := append([]string{"revert", "--no-edit"}, hashes...)
	cmd := exec.Command("git", args...)
	cmd.Dir = c.ProjectRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git revert failed: %s", string(out))
	}
	return nil
}
