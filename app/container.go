package app

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/david22573/codepicker/adapters/agent"
	ctxAdapters "github.com/david22573/codepicker/adapters/context"
	"github.com/david22573/codepicker/adapters/policy"
	"github.com/david22573/codepicker/adapters/tools"
	"github.com/david22573/codepicker/adapters/verifier"
	domainAgent "github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/config"
	domainCtx "github.com/david22573/codepicker/domain/context"
	"github.com/david22573/codepicker/domain/event"
	"github.com/david22573/codepicker/infra/audit"
	infraConfig "github.com/david22573/codepicker/infra/config"
	"github.com/david22573/codepicker/infra/fs"
	"github.com/david22573/codepicker/infra/indexer"
	"github.com/david22573/codepicker/infra/llm"
	"github.com/david22573/codepicker/infra/logging"
	"github.com/david22573/codepicker/infra/ratelimit"
	"github.com/david22573/codepicker/infra/shell"
	"github.com/david22573/codepicker/infra/storage"
	"go.uber.org/zap"
)

// Container holds all the dependencies for the application.
type Container struct {
	Planner          *agent.Planner
	PlanExecutor     *agent.PlanExecutor
	Auditor          *agent.Auditor
	Explainer        *agent.Explainer
	TwoPassEngine    *agent.TwoPassEngine
	Verifier         *verifier.Pipeline
	ContextBuilder   *ctxAdapters.SmartBuilder
	ProjectPrimer    *ctxAdapters.ProjectPrimer
	Repository       *storage.SQLiteRepository
	SliceStore       domainCtx.SliceStore
	WorkspaceManager *fs.WorkspaceManager
	ShadowManager    *fs.ShadowManager
	IndexManager     *indexer.IndexManager
	EventBus         *event.DataBus
	Logger           *logging.Logger
	CostTracker      *llm.CostTracker
	RateLimiter      *ratelimit.ToolRateLimiter
	Config           *config.AppConfig

	// Lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
}

func NewContainer(apiKey, rootDir, modelOverride string, dryRun, ciMode, verbose bool) (*Container, error) {
	// 1. Initialize Logger
	env := "development"
	if ciMode {
		env = "production"
	}
	logger, err := logging.NewLogger(env, verbose)
	if err != nil {
		return nil, err
	}

	// 2. Load Configuration
	cfgPath := filepath.Join(rootDir, "codepicker.yaml")
	loader := infraConfig.NewLoader(cfgPath)
	cfg, err := loader.Load()
	if err != nil {
		logger.Warn("Failed to load configuration file, using defaults", zap.Error(err))
		cfg = config.DefaultConfig()
	}

	if modelOverride != "" {
		cfg.LLM.Model = modelOverride
	}
	if dryRun {
		cfg.Environment = "dry-run"
	}

	// 3. Infrastructure
	dbPath := filepath.Join(rootDir, ".codepicker", "state.db")
	repo, err := storage.NewSQLiteRepository(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to init sqlite: %w", err)
	}

	workspaceMgr := fs.NewWorkspaceManager(rootDir)
	eventBus := event.NewDataBus()

	costTracker := llm.NewCostTracker(cfg.LLM.InputCostPer1M, cfg.LLM.OutputCostPer1M)
	_ = llm.NewBudgetGuard(costTracker, cfg.LLM.BudgetCap)

	llmClient := llm.NewOpenRouterAdapter(
		apiKey,
		cfg.LLM.Model,
		time.Duration(cfg.LLM.TimeoutSeconds)*time.Second,
	)

	embedClient := llm.NewEmbeddingClient(apiKey, cfg.Embedding.Model)

	shadowMgr := fs.NewShadowManager(rootDir, dryRun)
	shellExec := shell.NewExecutor(30*time.Second, 5000, dryRun, rootDir)

	allTools := tools.DefaultSet(shadowMgr, shellExec, rootDir, embedClient, repo)
	rateLimiter := ratelimit.NewToolRateLimiter(20)

	// 4. Policy
	var guardRail domainAgent.Policy
	if ciMode {
		guardRail = policy.NewStrictPolicy(dryRun, ciMode)
	} else {
		policyConfig, _ := policy.LoadPolicy(filepath.Join(rootDir, "policy.json"))
		guardRail = policy.NewEnforcer(*policyConfig, dryRun)
	}

	// 5. Agents
	worker := agent.NewReActAgent(
		llmClient,
		allTools,
		eventBus,
		logger,
		guardRail,
		costTracker,
		rateLimiter,
		cfg.LLM.BudgetCap,
	)
	worker.SetVerbose(verbose)

	planner := agent.NewPlanner(worker)

	executor := agent.NewPlanExecutor(worker, repo, workspaceMgr, shadowMgr)

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

	explainer := agent.NewExplainer(llmClient, repo)

	// 6. Two-Pass Engine
	packedContent := ""
	twoPass := agent.NewTwoPassEngine(
		llmClient,
		repo,
		allTools,
		guardRail,
		logger,
		costTracker,
		rateLimiter,
		packedContent,
	)

	// 7. Context & Verification
	slicer := indexer.NewCodeSlicer()
	indexManager := indexer.NewIndexManager(slicer, repo, embedClient)
	reranker := ctxAdapters.NewReranker(llmClient)
	ctxBuilder := ctxAdapters.NewSmartBuilder(repo, embedClient, reranker, cfg.Agent.MaxContextSize)
	primer := ctxAdapters.NewProjectPrimer(rootDir)
	verifierPipeline := verifier.NewPipeline(rootDir)

	// Create container context for lifecycle management
	ctx, cancel := context.WithCancel(context.Background())

	// 8. Background Cleanup (Managed)
	go func() {
		// Wait for shutdown or run cleanup
		select {
		case <-ctx.Done():
			return // Container closing, skip cleanup
		default:
			cleanupPolicy := audit.CleanupPolicy{
				MaxAge:   30 * 24 * time.Hour,
				MaxCount: 100,
			}
			auditDir := filepath.Join(rootDir, ".codepicker", "audit")
			// We execute cleanup immediately but check context inside if strictly needed,
			// or here we rely on the fact that it's a short operation.
			_ = audit.CleanupAudits(auditDir, cleanupPolicy)
		}
	}()

	return &Container{
		Planner:          planner,
		PlanExecutor:     executor,
		Auditor:          auditor,
		Explainer:        explainer,
		TwoPassEngine:    twoPass,
		Verifier:         verifierPipeline,
		ContextBuilder:   ctxBuilder,
		ProjectPrimer:    primer,
		WorkspaceManager: workspaceMgr,
		ShadowManager:    shadowMgr,
		Repository:       repo,
		SliceStore:       repo,
		IndexManager:     indexManager,
		EventBus:         eventBus,
		Logger:           logger,
		CostTracker:      costTracker,
		RateLimiter:      rateLimiter,
		Config:           cfg,
		ctx:              ctx,
		cancel:           cancel,
	}, nil
}

func (c *Container) Close() {
	// Signal background tasks to stop
	if c.cancel != nil {
		c.cancel()
	}

	if c.Logger != nil {
		_ = c.Logger.Sync()
	}
	if c.Repository != nil {
		_ = c.Repository.Close()
	}
	if c.EventBus != nil {
		c.EventBus.Close()
	}
}
