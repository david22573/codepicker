package app

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/david22573/codepicker/adapters/agent"
	ctxAdapters "github.com/david22573/codepicker/adapters/context"
	"github.com/david22573/codepicker/adapters/policy"
	"github.com/david22573/codepicker/adapters/tools"
	domainAgent "github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/config"
	"github.com/david22573/codepicker/domain/event"
	"github.com/david22573/codepicker/infra/fs"
	"github.com/david22573/codepicker/infra/llm"
	"github.com/david22573/codepicker/infra/logging"
	"github.com/david22573/codepicker/infra/ratelimit"
	"github.com/david22573/codepicker/infra/shell"
	"github.com/david22573/codepicker/infra/storage"
)

func NewLLMStack(apiKey string, cfg *config.AppConfig) (*llm.OpenRouterAdapter, *llm.CostTracker, *llm.EmbeddingClient, error) {
	costTracker := llm.NewCostTracker(cfg.LLM.InputCostPer1M, cfg.LLM.OutputCostPer1M)
	llmClient := llm.NewOpenRouterAdapter(
		apiKey,
		cfg.LLM.Model,
		time.Duration(cfg.LLM.TimeoutSeconds)*time.Second,
	)
	embedClient := llm.NewEmbeddingClient(apiKey, cfg.Embedding.Model)
	return llmClient, costTracker, embedClient, nil
}

func NewStorageStack(rootDir string, dryRun bool) (*storage.SQLiteRepository, *fs.WorkspaceManager, *fs.ShadowManager, error) {
	dbPath := filepath.Join(rootDir, ".codepicker", "state.db")
	repo, err := storage.NewSQLiteRepository(dbPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to init sqlite: %w", err)
	}
	workspaceMgr := fs.NewWorkspaceManager(rootDir)
	shadowMgr := fs.NewShadowManager(rootDir, dryRun)
	return repo, workspaceMgr, shadowMgr, nil
}

func NewAgentStack(
	cfg *config.AppConfig,
	llmClient *llm.OpenRouterAdapter,
	costTracker *llm.CostTracker,
	repo *storage.SQLiteRepository,
	workspaceMgr *fs.WorkspaceManager,
	shadowMgr *fs.ShadowManager,
	embedClient *llm.EmbeddingClient,
	eventBus *event.DataBus,
	logger *logging.Logger,
	rootDir string,
	dryRun bool,
	ciMode bool,
	verbose bool,
) (*agent.ReActAgent, *agent.Planner, *agent.PlanExecutor, *agent.Auditor, *agent.Explainer, *agent.TwoPassEngine, *ctxAdapters.Reranker, error) {

	shellExec := shell.NewExecutor(30*time.Second, 5000, dryRun, rootDir)
	allTools := tools.DefaultSet(shadowMgr, shellExec, rootDir, embedClient, repo)
	rateLimiter := ratelimit.NewToolRateLimiter(20)

	var guardRail domainAgent.Policy
	if ciMode {
		guardRail = policy.NewStrictPolicy(dryRun, ciMode)
	} else {
		policyConfig, _ := policy.LoadPolicy(filepath.Join(rootDir, "policy.json"))
		guardRail = policy.NewEnforcer(*policyConfig, dryRun)
	}

	worker := agent.NewReActAgent(
		llmClient,
		allTools,
		eventBus,
		logger,
		guardRail,
		costTracker,
		rateLimiter,
		cfg.LLM.BudgetCap,
		cfg.Agent.MaxTurns,
	)
	worker.SetVerbose(verbose)

	planner := agent.NewPlanner(llmClient)
	executor := agent.NewPlanExecutor(worker, repo, workspaceMgr, shadowMgr, logger)

	auditor := agent.NewAuditor(
		llmClient,
		repo,
		allTools,
		guardRail,
		logger,
		costTracker,
		rateLimiter,
		eventBus,
		cfg.LLM.BudgetCap,
	)

	explainer := agent.NewExplainer(llmClient, repo, costTracker, cfg.LLM.BudgetCap)

	twoPass := agent.NewTwoPassEngine(
		llmClient,
		repo,
		allTools,
		guardRail,
		logger,
		costTracker,
		rateLimiter,
		cfg.LLM.BudgetCap,
		"",
	)

	reranker := ctxAdapters.NewReranker(llmClient, costTracker, cfg.LLM.BudgetCap)

	return worker, planner, executor, auditor, explainer, twoPass, reranker, nil
}