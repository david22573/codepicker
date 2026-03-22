package app

import (
	"fmt"
	"os"
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
	"github.com/david22573/codepicker/infra/indexer"
	"github.com/david22573/codepicker/infra/llm"
	"github.com/david22573/codepicker/infra/logging"
	"github.com/david22573/codepicker/infra/pathutil"
	"github.com/david22573/codepicker/infra/ratelimit"
	"github.com/david22573/codepicker/infra/storage"
	"go.uber.org/zap"
)

type AgentStackOpts struct {
	Config       *config.AppConfig
	LLMClient    llm.Provider
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
	Mapper       *indexer.RepoMapper
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
	dbPath := pathutil.GetStateDBPath(rootDir)
	repo, err := storage.NewSQLiteRepository(dbPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to init sqlite: %w", err)
	}
	workspaceMgr := fs.NewWorkspaceManager(rootDir)
	shadowMgr := fs.NewShadowManager(rootDir, dryRun)
	return repo, workspaceMgr, shadowMgr, nil
}

func NewAgentStack(opts AgentStackOpts, toolsOverride []domainAgent.Tool) (*agent.ReActAgent, *agent.Planner, *agent.PlanExecutor, *agent.Auditor, *agent.Explainer, *agent.TwoPassEngine, *ctxAdapters.SmartBuilder, error) {

	rateLimiter := ratelimit.NewToolRateLimiter(20)

	var guardRail domainAgent.Policy
	if opts.CIMode {
		guardRail = policy.NewStrictPolicy(opts.DryRun, opts.CIMode)
	} else {
		policyConfig, _ := policy.LoadPolicy(filepath.Join(opts.RootDir, "policy.json"))
		guardRail = policy.NewEnforcer(*policyConfig, opts.DryRun)
	}

	cacheDir := filepath.Join(opts.RootDir, ".codepicker", "cache")
	enableCaching := opts.CIMode || opts.DryRun
	cachedLLM := llm.NewCachedAdapter(opts.LLMClient, cacheDir, enableCaching)

	backpressuredLLM := llm.NewBackpressureAdapter(cachedLLM, 5, 30*time.Second)
	toolPool := agent.NewBoundedWorkerPool(10)

	workerNodeURL := os.Getenv("CODEPICKER_WORKER_URL")
	if workerNodeURL != "" {
		toolsOverride = tools.MapDistributed(toolsOverride, workerNodeURL, []string{"run_cmd"})
		opts.Logger.Info("Distributed tool execution enabled", zap.String("worker_url", workerNodeURL))
	}

	worker := agent.NewReActAgent(
		backpressuredLLM,
		toolsOverride,
		opts.EventBus,
		opts.Logger,
		guardRail,
		opts.CostTracker,
		rateLimiter,
		toolPool,
		opts.Config.LLM.BudgetCap,
		opts.Config.Agent.MaxTurns,
	)
	worker.SetVerbose(opts.Verbose)

	planner := agent.NewPlanner(backpressuredLLM)
	executor := agent.NewPlanExecutor(worker, opts.Repo, opts.WorkspaceMgr, opts.ShadowMgr, opts.Logger)

	auditor := agent.NewAuditor(
		backpressuredLLM,
		opts.Repo,
		toolsOverride,
		guardRail,
		opts.Logger,
		opts.CostTracker,
		rateLimiter,
		toolPool,
		opts.EventBus,
		opts.Config.LLM.BudgetCap,
	)

	explainer := agent.NewExplainer(backpressuredLLM, opts.Repo, opts.CostTracker, opts.Config.LLM.BudgetCap)

	twoPass := agent.NewTwoPassEngine(
		backpressuredLLM,
		opts.Repo,
		toolsOverride,
		guardRail,
		opts.Logger,
		opts.CostTracker,
		rateLimiter,
		toolPool,
		opts.Config.LLM.BudgetCap,
		"",
	)

	reranker := ctxAdapters.NewReranker(backpressuredLLM, opts.CostTracker, opts.Config.LLM.BudgetCap, opts.Mapper)
	ctxBuilder := ctxAdapters.NewSmartBuilder(opts.Repo, opts.EmbedClient, reranker, opts.ShadowMgr, opts.Config.Agent.MaxContextSize, opts.Mapper)

	return worker, planner, executor, auditor, explainer, twoPass, ctxBuilder, nil
}
