package app

import (
	"path/filepath"
	"time"

	"github.com/david22573/codepicker/adapters/agent"
	"github.com/david22573/codepicker/adapters/context"
	"github.com/david22573/codepicker/adapters/policy"
	"github.com/david22573/codepicker/adapters/tools"
	"github.com/david22573/codepicker/adapters/verifier"
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
	CostTracker      *llm.CostTracker // Phase 3: Exposed for CLI summaries
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

	logger.Info("Initializing CodePicker Container",
		zap.String("mode", logEnv),
		zap.String("root", projectRoot))

	hiddenDir := filepath.Join(projectRoot, ".codepicker")
	dbPath := filepath.Join(hiddenDir, "state.db")

	repo, err := storage.NewSQLiteRepository(dbPath)
	if err != nil {
		logger.Error("Database initialization failed", zap.Error(err))
		return nil, err
	}

	workspaceMgr := fs.NewWorkspaceManager(projectRoot)
	gitClient := git.NewClient(projectRoot)

	// 2. Initialize LLM & Cost Tracker
	selectedModel := "moonshotai/kimi-k2.5"
	if llmModel != "" {
		selectedModel = llmModel
	}
	llmClient := llm.NewOpenRouterAdapter(apiKey, selectedModel)

	// Phase 3: Initialize Cost Tracker (Estimates for Kimi/Moonshot: ~$0.30 input / $0.60 output)
	costTracker := llm.NewCostTracker(0.3, 0.6)

	// 3. Initialize Infrastructure
	shadowMgr := fs.NewShadowManager(projectRoot)
	shellExec := shell.NewExecutor(30*time.Second, 5000)
	allTools := tools.DefaultSet(shadowMgr, shellExec, projectRoot)

	// 4. Initialize Security Policy
	policyPath := filepath.Join(projectRoot, "policy.json")
	policyConfig, _ := policy.LoadPolicy(policyPath)
	guardRail := policy.NewEnforcer(*policyConfig, isDryRun)

	ctxBuilder := context.NewSliceBasedBuilder(repo, 16000)

	// 5. Initialize Agents with Dependencies
	// Note: We inject costTracker into the worker so it can record usage per turn
	worker := agent.NewReActAgent(llmClient, allTools, guardRail, repo, logger, costTracker)
	planner := agent.NewPlanner(llmClient, repo)

	// Note: We inject workspaceMgr into the executor to enable Transaction/Rollback support
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
