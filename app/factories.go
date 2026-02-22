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

type AgentStackOpts struct {
	Config       *config.AppConfig
	LLMClient    *llm.OpenRouterAdapter
	CostTracker  *llm.CostTracker
	Repo         *storage.SQLiteRepository
	WorkspaceMgr *fs.WorkspaceManager
	ShadowMgr    *fs.ShadowManager
	EmbedClient  *llm.EmbeddingClient
	EventBus     *event.DataBus
	Logger       *logging.Logger
	RootDir      string
	DryRun       bool
	CIMode       bool
	Verbose      bool
}

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

func NewAgentStack(opts AgentStackOpts) (*agent.ReActAgent, *agent.Planner, *agent.PlanExecutor, *agent.Auditor, *agent.Explainer, *agent.TwoPassEngine, *ctxAdapters.Reranker, error) {

	shellExec := shell.NewExecutor(30*time.Second, 5000, opts.DryRun, opts.RootDir)
	allTools := tools.DefaultSet(opts.ShadowMgr, shellExec, opts.RootDir, opts.EmbedClient, opts.Repo)
	rateLimiter := ratelimit.NewToolRateLimiter(20)

	var guardRail domainAgent.Policy
	if opts.CIMode {
		guardRail = policy.NewStrictPolicy(opts.DryRun, opts.CIMode)
	} else {
		policyConfig, _ := policy.LoadPolicy(filepath.Join(opts.RootDir, "policy.json"))
		guardRail = policy.NewEnforcer(*policyConfig, opts.DryRun)
	}

	worker := agent.NewReActAgent(
		opts.LLMClient,
		allTools,
		opts.EventBus,
		opts.Logger,
		guardRail,
		opts.CostTracker,
		rateLimiter,
		opts.Config.LLM.BudgetCap,
		opts.Config.Agent.MaxTurns,
	)
	worker.SetVerbose(opts.Verbose)

	planner := agent.NewPlanner(opts.LLMClient)
	executor := agent.NewPlanExecutor(worker, opts.Repo, opts.WorkspaceMgr, opts.ShadowMgr, opts.Logger)

	auditor := agent.NewAuditor(
		opts.LLMClient,
		opts.Repo,
		allTools,
		guardRail,
		opts.Logger,
		opts.CostTracker,
		rateLimiter,
		opts.EventBus,
		opts.Config.LLM.BudgetCap,
	)

	explainer := agent.NewExplainer(opts.LLMClient, opts.Repo, opts.CostTracker, opts.Config.LLM.BudgetCap)

	twoPass := agent.NewTwoPassEngine(
		opts.LLMClient,
		opts.Repo,
		allTools,
		guardRail,
		opts.Logger,
		opts.CostTracker,
		rateLimiter,
		opts.Config.LLM.BudgetCap,
		"",
	)

	reranker := ctxAdapters.NewReranker(opts.LLMClient, opts.CostTracker, opts.Config.LLM.BudgetCap)

	return worker, planner, executor, auditor, explainer, twoPass, reranker, nil
}