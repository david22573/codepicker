package app

import (
	"path/filepath"
	"time"

	"github.com/david22573/codepicker/adapters/agent"
	"github.com/david22573/codepicker/adapters/context"
	"github.com/david22573/codepicker/adapters/policy"
	"github.com/david22573/codepicker/adapters/tools"
	"github.com/david22573/codepicker/adapters/verifier"
	"github.com/david22573/codepicker/domain/config" // Import Config
	contextDomain "github.com/david22573/codepicker/domain/context"
	"github.com/david22573/codepicker/infra/fs"
	"github.com/david22573/codepicker/infra/git"
	"github.com/david22573/codepicker/infra/llm"
	"github.com/david22573/codepicker/infra/logging"
	"github.com/david22573/codepicker/infra/shell"
	"github.com/david22573/codepicker/infra/storage"
	"go.uber.org/zap"
)

type Container struct {
	Planner          *agent.Planner
	PlanExecutor     *agent.PlanExecutor
	Auditor          *agent.Auditor
	Explainer        *agent.Explainer
	TwoPassEngine    *agent.TwoPassEngine
	Verifier         *verifier.Pipeline
	Git              *git.Client
	ContextBuilder   *context.SliceBasedBuilder
	WorkspaceManager *fs.WorkspaceManager
	Repository       *storage.SQLiteRepository
	SliceStore       contextDomain.SliceStore
	Logger           *logging.Logger
	CostTracker      *llm.CostTracker
	Config           *config.AppConfig // Exposed for CLI usage
}

func NewContainer(apiKey, projectRoot, llmModel string, isDryRun, isCI bool) (*Container, error) {
	// 1. Initialize Logger
	logEnv := "development"
	if isCI {
		logEnv = "production"
	}

	logger, err := logging.NewLogger(logEnv)
	if err != nil {
		return nil, err
	}

	// 2. Load Configuration
	configPath := filepath.Join(projectRoot, "config.json")
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		logger.Warn("Failed to load config.json, using defaults", zap.Error(err))
		cfg = config.DefaultConfig()
	}

	// Override model if provided via CLI
	if llmModel != "" {
		cfg.LLM.Model = llmModel
	}

	logger.Info("Initializing CodePicker Container",
		zap.String("mode", logEnv),
		zap.String("root", projectRoot),
		zap.String("model", cfg.LLM.Model),
		zap.Bool("dry_run", isDryRun))

	hiddenDir := filepath.Join(projectRoot, ".codepicker")
	dbPath := filepath.Join(hiddenDir, "state.db")

	repo, err := storage.NewSQLiteRepository(dbPath)
	if err != nil {
		logger.Error("Database initialization failed", zap.Error(err))
		return nil, err
	}

	workspaceMgr := fs.NewWorkspaceManager(projectRoot)
	gitClient := git.NewClient(projectRoot, isDryRun)

	// 3. Initialize LLM & Cost Tracker with Config
	// Note: You'll need to update NewOpenRouterAdapter signature in infra/llm next!
	llmClient := llm.NewOpenRouterAdapter(apiKey, cfg.LLM.Model, time.Duration(cfg.LLM.TimeoutSeconds)*time.Second)

	// Initialize Cost Tracker with limits
	costTracker := llm.NewCostTracker(cfg.LLM.InputCostPer1M, cfg.LLM.OutputCostPer1M)

	// 4. Initialize Infrastructure
	shadowMgr := fs.NewShadowManager(projectRoot)
	shellExec := shell.NewExecutor(30*time.Second, 5000, isDryRun)
	allTools := tools.DefaultSet(shadowMgr, shellExec, projectRoot)

	// 5. Initialize Security Policy
	policyPath := filepath.Join(projectRoot, "policy.json")
	policyConfig, _ := policy.LoadPolicy(policyPath)
	guardRail := policy.NewEnforcer(*policyConfig, isDryRun)

	ctxBuilder := context.NewSliceBasedBuilder(repo, cfg.Agent.MaxContextSize)

	// 6. Initialize Agents
	// We inject costTracker into the worker so it can record usage per turn
	worker := agent.NewReActAgent(llmClient, allTools, guardRail, repo, logger, costTracker)
	planner := agent.NewPlanner(llmClient, repo)

	executor := agent.NewPlanExecutor(worker, repo, workspaceMgr)
	auditor := agent.NewAuditor(llmClient, repo, allTools, guardRail)
	explainer := agent.NewExplainer(llmClient, repo)
	twoPass := agent.NewTwoPassEngine(llmClient, repo, allTools, guardRail)
	verifier := verifier.NewPipeline(projectRoot)

	return &Container{
		Planner:          planner,
		PlanExecutor:     executor,
		Auditor:          auditor,
		Explainer:        explainer,
		TwoPassEngine:    twoPass,
		Verifier:         verifier,
		Git:              gitClient,
		ContextBuilder:   ctxBuilder,
		WorkspaceManager: workspaceMgr,
		Repository:       repo,
		SliceStore:       repo,
		Logger:           logger,
		CostTracker:      costTracker,
		Config:           cfg,
	}, nil
}

func (c *Container) Close() {
	if c.Logger != nil {
		c.Logger.Info("Shutting down container...")
		c.Logger.Sync()
	}
	if c.Repository != nil {
		c.Repository.Close()
	}
}
