package app

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/david22573/codepicker/adapters/agent"
	contextBuilder "github.com/david22573/codepicker/adapters/context"
	"github.com/david22573/codepicker/adapters/policy"
	"github.com/david22573/codepicker/adapters/tools"
	"github.com/david22573/codepicker/infra/fs"
	"github.com/david22573/codepicker/infra/llm"
	"github.com/david22573/codepicker/infra/shell"
	"github.com/david22573/codepicker/infra/storage"
)

// Container holds the wired dependencies
type Container struct {
	Planner        *agent.Planner
	PlanExecutor   *agent.PlanExecutor
	ContextBuilder *contextBuilder.Builder
}

// NewContainer initializes the entire application stack
func NewContainer(apiKey, projectRoot string, isDryRun, isCI bool) (*Container, error) {
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
	llmClient := llm.NewOpenRouterAdapter(apiKey, "liquid/lfm-2.5-1.2b-thinking:free")

	// Filesystem & Shell
	shadowMgr := fs.NewShadowManager(projectRoot)
	shellExec := shell.NewExecutor(30*time.Second, 5000) // 30s timeout, 5KB output limit

	// 3. Initialize Adapters (Tools & Policy)
	toolSet := tools.DefaultSet(shadowMgr, shellExec, projectRoot)
	strictPolicy := policy.NewStrictPolicy(isDryRun, isCI)

	// 4. Initialize Worker (ReAct Agent)
	worker := agent.NewReActAgent(llmClient, toolSet, strictPolicy, repo)

	// 5. Initialize Planner & Executor
	planner := agent.NewPlanner(llmClient)
	executor := agent.NewPlanExecutor(worker, repo)

	ctxBuilder := contextBuilder.NewBuilder()

	return &Container{
		Planner:        planner,
		PlanExecutor:   executor,
		ContextBuilder: ctxBuilder,
	}, nil
}
