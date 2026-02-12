package app

import (
	"path/filepath"
	"time"

	"github.com/david22573/codepicker/adapters/agent"
	"github.com/david22573/codepicker/adapters/context"
	"github.com/david22573/codepicker/adapters/policy"
	"github.com/david22573/codepicker/adapters/tools"
	"github.com/david22573/codepicker/adapters/verifier"
	"github.com/david22573/codepicker/domain/config"
	domainCtx "github.com/david22573/codepicker/domain/context"
	"github.com/david22573/codepicker/domain/event"
	"github.com/david22573/codepicker/infra/fs"
	"github.com/david22573/codepicker/infra/llm"
	"github.com/david22573/codepicker/infra/logging"
	"github.com/david22573/codepicker/infra/shell"
	"github.com/david22573/codepicker/infra/storage"
)

type Container struct {
	Planner          *agent.Planner
	PlanExecutor     *agent.PlanExecutor
	Auditor          *agent.Auditor
	Explainer        *agent.Explainer
	TwoPassEngine    *agent.TwoPassEngine // Required by cmd/fix.go
	Verifier         *verifier.Pipeline
	ContextBuilder   *context.SliceBasedBuilder
	ProjectPrimer    *context.ProjectPrimer
	Repository       *storage.SQLiteRepository
	SliceStore       domainCtx.SliceStore // Interface for flexibility
	WorkspaceManager *fs.WorkspaceManager
	EventBus         *event.DataBus
	Logger           *logging.Logger
	CostTracker      *llm.CostTracker
	Config           *config.AppConfig
}

func NewContainer(apiKey, projectRoot, llmModel string, isDryRun, isCI bool) (*Container, error) {
	// 1. Core Infrastructure
	logger, _ := logging.NewLogger("development")
	if isCI {
		logger, _ = logging.NewLogger("production")
	}
	cfg, _ := config.LoadConfig(filepath.Join(projectRoot, "config.json"))
	if llmModel != "" {
		cfg.LLM.Model = llmModel
	}

	dbPath := filepath.Join(projectRoot, ".codepicker", "state.db")
	repo, err := storage.NewSQLiteRepository(dbPath)
	if err != nil {
		return nil, err
	}

	workspaceMgr := fs.NewWorkspaceManager(projectRoot)
	eventBus := event.NewDataBus()

	// 2. LLM & Cost Safety
	llmClient := llm.NewOpenRouterAdapter(apiKey, cfg.LLM.Model, time.Duration(cfg.LLM.TimeoutSeconds)*time.Second)
	costTracker := llm.NewCostTracker(cfg.LLM.InputCostPer1M, cfg.LLM.OutputCostPer1M)
	_ = llm.NewBudgetGuard(costTracker, cfg.LLM.BudgetCap)

	// 3. Tools & Policy
	shadowMgr := fs.NewShadowManager(projectRoot)
	shellExec := shell.NewExecutor(30*time.Second, 5000, isDryRun)
	allTools := tools.DefaultSet(shadowMgr, shellExec, projectRoot)

	policyConfig, _ := policy.LoadPolicy(filepath.Join(projectRoot, "policy.json"))
	guardRail := policy.NewEnforcer(*policyConfig, isDryRun)

	// 4. Agents
	worker := agent.NewReActAgent(llmClient, allTools, eventBus, logger, costTracker, cfg.LLM.BudgetCap)
	planner := agent.NewPlanner(llmClient, repo)
	executor := agent.NewPlanExecutor(worker, repo, workspaceMgr)

	auditor := agent.NewAuditor(llmClient, repo, allTools, guardRail, logger, costTracker)
	explainer := agent.NewExplainer(llmClient, repo)
	twoPass := agent.NewTwoPassEngine(llmClient, repo, allTools, guardRail, logger, costTracker)

	// 5. Context
	ctxBuilder := context.NewSliceBasedBuilder(repo, cfg.Agent.MaxContextSize)
	primer := context.NewProjectPrimer(projectRoot)
	verifierPipeline := verifier.NewPipeline(projectRoot)

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
		Repository:       repo,
		SliceStore:       repo, // Repo satisfies SliceStore
		EventBus:         eventBus,
		Logger:           logger,
		CostTracker:      costTracker,
		Config:           cfg,
	}, nil
}

func (c *Container) Close() {
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
