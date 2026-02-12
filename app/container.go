package app

import (
	"os"
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
	"github.com/david22573/codepicker/infra/audit"
	infraConfig "github.com/david22573/codepicker/infra/config"
	"github.com/david22573/codepicker/infra/fs"
	"github.com/david22573/codepicker/infra/llm"
	"github.com/david22573/codepicker/infra/logging"
	"github.com/david22573/codepicker/infra/ratelimit"
	"github.com/david22573/codepicker/infra/shell"
	"github.com/david22573/codepicker/infra/storage"
	"go.uber.org/zap"
)

// Container holds all the dependencies for the application.
// It acts as the central dependency injection root.
type Container struct {
	// Agents
	Planner       *agent.Planner
	PlanExecutor  *agent.PlanExecutor
	Auditor       *agent.Auditor
	Explainer     *agent.Explainer
	TwoPassEngine *agent.TwoPassEngine
	Verifier      *verifier.Pipeline

	// Context & Data
	ContextBuilder   *context.SliceBasedBuilder
	ProjectPrimer    *context.ProjectPrimer
	Repository       *storage.SQLiteRepository
	SliceStore       domainCtx.SliceStore
	WorkspaceManager *fs.WorkspaceManager
	ShadowManager    *fs.ShadowManager // Exposed for CLI usage

	// Infrastructure
	EventBus    *event.DataBus
	Logger      *logging.Logger
	CostTracker *llm.CostTracker
	RateLimiter *ratelimit.ToolRateLimiter
	Config      *config.AppConfig
}

// NewContainer initializes the application container with all dependencies wired up.
func NewContainer(apiKey, projectRoot, llmModel string, isDryRun, isCI bool) (*Container, error) {
	// 1. Initialize Logger
	logger, _ := logging.NewLogger("development")
	if isCI {
		logger, _ = logging.NewLogger("production")
	}

	// 2. Load Configuration
	configPath := filepath.Join(projectRoot, "configs", "default.yaml")
	if customPath := os.Getenv("CODEPICKER_CONFIG"); customPath != "" {
		configPath = customPath
	}

	loader := infraConfig.NewLoader(configPath)
	cfg, err := loader.Load()
	if err != nil {
		logger.Warn("Failed to load configuration file, using defaults", zap.Error(err))
		cfg = config.DefaultConfig()
	}

	// Apply CLI overrides
	if llmModel != "" {
		cfg.LLM.Model = llmModel
	}
	if isDryRun {
		cfg.Environment = "dry-run"
	}

	// 3. Persistence Layer
	dbPath := filepath.Join(projectRoot, ".codepicker", "state.db")
	repo, err := storage.NewSQLiteRepository(dbPath)
	if err != nil {
		return nil, err
	}

	workspaceMgr := fs.NewWorkspaceManager(projectRoot)
	eventBus := event.NewDataBus()

	// 4. LLM & Cost Safety
	llmClient := llm.NewOpenRouterAdapter(
		apiKey,
		cfg.LLM.Model,
		time.Duration(cfg.LLM.TimeoutSeconds)*time.Second,
	)

	costTracker := llm.NewCostTracker(cfg.LLM.InputCostPer1M, cfg.LLM.OutputCostPer1M)
	_ = llm.NewBudgetGuard(costTracker, cfg.LLM.BudgetCap)

	// 5. Tools, Policy & Rate Limiting

	// Feature 5: Inject isDryRun into ShadowManager
	shadowMgr := fs.NewShadowManager(projectRoot, isDryRun)

	shellExec := shell.NewExecutor(30*time.Second, 5000, isDryRun, projectRoot)
	allTools := tools.DefaultSet(shadowMgr, shellExec, projectRoot)
	rateLimiter := ratelimit.NewToolRateLimiter(20)

	policyConfig, _ := policy.LoadPolicy(filepath.Join(projectRoot, "policy.json"))
	guardRail := policy.NewEnforcer(*policyConfig, isDryRun)

	// 6. Agents
	worker := agent.NewReActAgent(
		llmClient,
		allTools,
		eventBus,
		logger,
		costTracker,
		rateLimiter,
		cfg.LLM.BudgetCap,
	)

	planner := agent.NewPlanner(llmClient, repo)
	executor := agent.NewPlanExecutor(worker, repo, workspaceMgr)

	auditor := agent.NewAuditor(
		llmClient,
		repo,
		allTools,
		guardRail,
		logger,
		costTracker,
		rateLimiter,
	)

	explainer := agent.NewExplainer(llmClient, repo)

	packedContent := ""
	if content, err := os.ReadFile(filepath.Join(projectRoot, "codepicker_context.txt")); err == nil {
		packedContent = string(content)
	}

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

	// 7. Context Management
	ctxBuilder := context.NewSliceBasedBuilder(repo, cfg.Agent.MaxContextSize)
	primer := context.NewProjectPrimer(projectRoot)
	verifierPipeline := verifier.NewPipeline(projectRoot)

	// 8. Background Maintenance
	go func() {
		cleanupPolicy := audit.CleanupPolicy{
			MaxAge:   30 * 24 * time.Hour,
			MaxCount: 100,
		}
		auditDir := filepath.Join(projectRoot, ".codepicker", "audit")
		_ = audit.CleanupAudits(auditDir, cleanupPolicy)
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
		ShadowManager:    shadowMgr, // Exposed for CLI usage
		Repository:       repo,
		SliceStore:       repo,
		EventBus:         eventBus,
		Logger:           logger,
		CostTracker:      costTracker,
		RateLimiter:      rateLimiter,
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
