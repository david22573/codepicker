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
	domainAgent "github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/infra/fs"
	"github.com/david22573/codepicker/infra/llm"
	"github.com/david22573/codepicker/infra/shell"
	"github.com/david22573/codepicker/infra/storage"
)

// Container holds the wired dependencies
type Container struct {
	Planner        *agent.Planner
	PlanExecutor   *agent.PlanExecutor
	Auditor        *agent.Auditor
	Explainer      *agent.Explainer // <--- Phase 4: Added Explainer
	ContextBuilder *contextBuilder.Builder
	Repository     domainAgent.Repository
}

// NewContainer initializes the entire application stack
func NewContainer(apiKey, projectRoot, llmModel string, isDryRun, isCI bool) (*Container, error) {
	// ... [Infrastructure setup remains same] ...
	hiddenDir := filepath.Join(projectRoot, ".codepicker")
	if err := os.MkdirAll(hiddenDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create .codepicker dir: %w", err)
	}
	dbPath := filepath.Join(hiddenDir, "state.db")
	repo, err := storage.NewSQLiteRepository(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to init sqlite: %w", err)
	}

	selectedModel := "liquid/lfm-2.5-1.2b-thinking:free"
	if llmModel != "" {
		selectedModel = llmModel
	}
	llmClient := llm.NewOpenRouterAdapter(apiKey, selectedModel)

	shadowMgr := fs.NewShadowManager(projectRoot)
	shellExec := shell.NewExecutor(30*time.Second, 5000)

	allTools := tools.DefaultSet(shadowMgr, shellExec, projectRoot)
	strictPolicy := policy.NewStrictPolicy(isDryRun, isCI)
	worker := agent.NewReActAgent(llmClient, allTools, strictPolicy, repo)

	planner := agent.NewPlanner(llmClient, repo)
	executor := agent.NewPlanExecutor(worker, repo)

	// Phase 1 Auditor
	auditPolicy := policy.NewStrictPolicy(true, false)
	var readTools []domainAgent.Tool
	for _, t := range allTools {
		if t.Name() != "write_file" && t.Name() != "run_cmd" {
			readTools = append(readTools, t)
		}
	}
	auditor := agent.NewAuditor(llmClient, repo, readTools, auditPolicy)

	// Phase 4: Explainer
	explainer := agent.NewExplainer(llmClient, repo)

	ctxBuilder := contextBuilder.NewBuilder()

	return &Container{
		Planner:        planner,
		PlanExecutor:   executor,
		Auditor:        auditor,
		Explainer:      explainer, // <--- Wired here
		ContextBuilder: ctxBuilder,
		Repository:     repo,
	}, nil
}
