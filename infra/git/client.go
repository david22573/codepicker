package git

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/david22573/codepicker/domain/audit"
)

// Client wraps git command line operations.
type Client struct {
	ProjectRoot string
}

func NewClient(root string) *Client {
	return &Client{ProjectRoot: root}
}

// StageAll runs 'git add .' to stage all changes.
func (c *Client) StageAll() error {
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = c.ProjectRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add failed: %s", string(out))
	}
	return nil
}

// Commit creates a new commit with the provenance message.
func (c *Client) Commit(p *audit.Provenance) (string, error) {
	msg := p.FormatCommitMessage()

	// We use the -m flag. For very large messages, passing via stdin is safer,
	// but for metadata this is sufficient.
	cmd := exec.Command("git", "commit", "-m", msg)
	cmd.Dir = c.ProjectRoot

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git commit failed: %s", string(out))
	}

	// Return the output (usually contains the short hash and subject)
	return strings.TrimSpace(string(out)), nil
}

// IsDirty checks if there are uncommitted changes (to prevent committing unintended files).
func (c *Client) IsDirty() bool {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = c.ProjectRoot
	out, _ := cmd.Output()
	return len(out) > 0
}
