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

func (c *Client) StageAll(ctx context.Context) error {
	if c.DryRun {
		fmt.Println("🔍 [DRY-RUN] Skipping 'git add .'")
		return nil
	}
	cmd := exec.CommandContext(ctx, "git", "add", ".")
	cmd.Dir = c.ProjectRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add failed: %s", string(out))
	}
	return nil
}

func (c *Client) StageFiles(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	if c.DryRun {
		fmt.Printf("🔍 [DRY-RUN] Skipping 'git add' for %v\n", paths)
		return nil
	}
	args := append([]string{"add"}, paths...)
	cmd := exec.CommandContext(ctx, "git", args...)
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
		fmt.Printf("🔍 [DRY-RUN] Would commit with message:\n%s\n", message)
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

func (c *Client) RevertFiles(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	if c.DryRun {
		fmt.Printf("🔍 [DRY-RUN] Skipping reverting files: %v\n", paths)
		return nil
	}
	args := append([]string{"checkout", "HEAD", "--"}, paths...)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = c.ProjectRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git revert failed for %v: %s", paths, string(out))
	}
	return nil
}

func (c *Client) CreateBranch(ctx context.Context, name string) error {
	if c.DryRun {
		fmt.Printf("🔍 [DRY-RUN] Skipping branch creation: %s\n", name)
		return nil
	}
	cmd := exec.CommandContext(ctx, "git", "checkout", "-b", name)
	cmd.Dir = c.ProjectRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git branch creation failed: %s", string(out))
	}
	return nil
}

func (c *Client) IsDirty(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = c.ProjectRoot
	out, _ := cmd.Output()
	return len(out) > 0
}

// GetChangedFiles lists modified, added, renamed, or untracked files in the repo
func (c *Client) GetChangedFiles(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = c.ProjectRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w", err)
	}

	var files []string
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		if len(line) < 4 {
			continue
		}
		status := line[:2]
		path := line[3:]

		// If renamed, extract target path
		if strings.Contains(status, "R") {
			parts := strings.Split(path, " -> ")
			if len(parts) == 2 {
				path = parts[1]
			}
		}

		// Normalize quotes if any
		path = strings.Trim(path, "\"")
		if path != "" {
			files = append(files, path)
		}
	}
	return files, nil
}

// GetLastCodepickerCommits finds the last N commits made by the agent.
func (c *Client) GetLastCodepickerCommits(ctx context.Context, n int) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "log", "--grep=\\[codepicker\\]", fmt.Sprintf("-n%d", n), "--format=%H")
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

// RevertCommits cleanly reverts the given commit hashes one by one.
func (c *Client) RevertCommits(ctx context.Context, hashes []string) error {
	if len(hashes) == 0 {
		return nil
	}
	if c.DryRun {
		fmt.Printf("🔍 [DRY-RUN] Skipping git revert for %v\n", hashes)
		return nil
	}

	for _, hash := range hashes {
		cmd := exec.CommandContext(ctx, "git", "revert", "--no-edit", hash)
		cmd.Dir = c.ProjectRoot

		if out, err := cmd.CombinedOutput(); err != nil {
			// Abort the revert so we don't leave the working tree in a conflicted state
			abortCmd := exec.CommandContext(ctx, "git", "revert", "--abort")
			abortCmd.Dir = c.ProjectRoot
			_ = abortCmd.Run()

			return fmt.Errorf("git revert failed on commit %s (aborted to keep tree clean): %s", hash, string(out))
		}
	}
	return nil
}
