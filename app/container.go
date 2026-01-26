package app

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/david22573/codepicker/adapters/agent"
	"github.com/david22573/codepicker/adapters/policy"
	"github.com/david22573/codepicker/adapters/tools"
	domainAgent "github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/infra/fs"
	"github.com/david22573/codepicker/infra/llm"
	"github.com/david22573/codepicker/infra/shell"
	"github.com/david22573/codepicker/infra/storage"
)

// Container holds the wired dependencies
type Container struct {
	Agent domainAgent.Agent
}

// NewContainer initializes the entire application stack
func NewContainer(apiKey string, projectRoot string) (*Container, error) {
	// 1. Ensure hidden directory exists for DB and Shadow
	hiddenDir := filepath.Join(projectRoot, ".codepicker")
	if err := os.MkdirAll(hiddenDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create .codepicker dir: %w", err)
	}

	// 2. Initialize Infrastructure
	// DB
	dbPath := filepath.Join(hiddenDir, "state.db")
	repo, err := storage.NewSQLiteRepository(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to init sqlite: %w", err)
	}

	// LLM
	llmClient := llm.NewOpenRouterAdapter(apiKey, "anthropic/claude-3.5-sonnet") // Defaulting to Sonnet for coding

	// Filesystem & Shell
	shadowMgr := fs.NewShadowManager(projectRoot)
	shellExec := shell.NewExecutor(30*time.Second, 5000) // 30s timeout, 5KB output limit

	// 3. Initialize Adapters (Tools & Policy)
	toolSet := tools.DefaultSet(shadowMgr, shellExec, projectRoot)
	strictPolicy := policy.NewStrictPolicy()

	// 4. Initialize Agent (The Brain)
	// We inject all the infrastructure components here
	reactAgent := agent.NewReActAgent(llmClient, toolSet, strictPolicy, repo)

	return &Container{
		Agent: reactAgent,
	}, nil
}
