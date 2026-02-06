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
	Repository       *storage.SQLiteRepository // Changed to pointer to avoid lock copy
	SliceStore       contextDomain.SliceStore
	Logger           *logging.Logger
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

	// Fixed: Use zap fields instead of map
	logger.Info("Initializing CodePicker Container",
		zap.String("mode", logEnv),
		zap.String("root", projectRoot))

	hiddenDir := filepath.Join(projectRoot, ".codepicker")
	dbPath := filepath.Join(hiddenDir, "state.db")

	repo, err := storage.NewSQLiteRepository(dbPath)
	if err != nil {
		// Fixed: Use zap fields
		logger.Error("Database initialization failed", zap.Error(err))
		return nil, err
	}

	workspaceMgr := fs.NewWorkspaceManager(projectRoot)
	_ = git.NewClient(projectRoot)

	selectedModel := "moonshotai/kimi-k2.5"
	if llmModel != "" {
		selectedModel = llmModel
	}
	llmClient := llm.NewOpenRouterAdapter(apiKey, selectedModel)

	shadowMgr := fs.NewShadowManager(projectRoot)
	shellExec := shell.NewExecutor(30*time.Second, 5000)
	allTools := tools.DefaultSet(shadowMgr, shellExec, projectRoot)

	policyPath := filepath.Join(projectRoot, "policy.json")
	policyConfig, _ := policy.LoadPolicy(policyPath)
	guardRail := policy.NewEnforcer(*policyConfig, isDryRun)

	ctxBuilder := context.NewSliceBasedBuilder(repo, 16000)

	// Updated to pass logger
	worker := agent.NewReActAgent(llmClient, allTools, guardRail, repo, logger)
	planner := agent.NewPlanner(llmClient, repo)
	executor := agent.NewPlanExecutor(worker, repo)

	return &Container{
		Planner:          planner,
		PlanExecutor:     executor,
		ContextBuilder:   ctxBuilder,
		WorkspaceManager: workspaceMgr,
		Repository:       repo, // Pass pointer, do not dereference
		SliceStore:       repo,
		Logger:           logger,
	}, nil
}

// Close ensures all resources (DB, Logger) are flushed and closed properly
func (c *Container) Close() {
	if c.Logger != nil {
		c.Logger.Info("Shutting down container...")
		c.Logger.Sync()
	}
	if c.Repository != nil {
		c.Repository.Close()
	}
}
