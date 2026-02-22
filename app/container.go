package app

import (
	"context"
	"path/filepath"
	"time"

	"github.com/david22573/codepicker/adapters/agent"
	ctxAdapters "github.com/david22573/codepicker/adapters/context"
	"github.com/david22573/codepicker/adapters/verifier"
	"github.com/david22573/codepicker/domain/config"
	domainCtx "github.com/david22573/codepicker/domain/context"
	"github.com/david22573/codepicker/domain/event"
	"github.com/david22573/codepicker/infra/audit"
	infraConfig "github.com/david22573/codepicker/infra/config"
	"github.com/david22573/codepicker/infra/fs"
	"github.com/david22573/codepicker/infra/indexer"
	"github.com/david22573/codepicker/infra/llm"
	"github.com/david22573/codepicker/infra/logging"
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
	Config           *config.AppConfig

	ctx    context.Context
	cancel context.CancelFunc
}

func NewContainer(apiKey, rootDir, modelOverride string, dryRun, ciMode, verbose bool) (*Container, error) {
	env := "development"
	if ciMode {
		env = "production"
	}
	logger, err := logging.NewLogger(env, verbose)
	if err != nil {
		return nil, err
	}

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

	llmClient, costTracker, embedClient, err := NewLLMStack(apiKey, cfg)
	if err != nil {
		return nil, err
	}

	repo, workspaceMgr, shadowMgr, err := NewStorageStack(rootDir, dryRun)
	if err != nil {
		return nil, err
	}

	eventBus := event.NewDataBus()

	_, planner, executor, auditor, explainer, twoPass, reranker, err := NewAgentStack(AgentStackOpts{
		Config:       cfg,
		LLMClient:    llmClient,
		CostTracker:  costTracker,
		Repo:         repo,
		WorkspaceMgr: workspaceMgr,
		ShadowMgr:    shadowMgr,
		EmbedClient:  embedClient,
		EventBus:     eventBus,
		Logger:       logger,
		RootDir:      rootDir,
		DryRun:       dryRun,
		CIMode:       ciMode,
		Verbose:      verbose,
	})
	if err != nil {
		return nil, err
	}

	slicer := indexer.NewCodeSlicer()
	indexManager := indexer.NewIndexManager(slicer, repo, embedClient)
	ctxBuilder := ctxAdapters.NewSmartBuilder(repo, embedClient, reranker, cfg.Agent.MaxContextSize)
	primer := ctxAdapters.NewProjectPrimer(rootDir)
	verifierPipeline := verifier.NewPipeline(rootDir)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		ticker := time.NewTicker(12 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return 
			case <-ticker.C:
				cleanupPolicy := audit.CleanupPolicy{
					MaxAge:   30 * 24 * time.Hour,
					MaxCount: 100,
				}
				auditDir := filepath.Join(rootDir, ".codepicker", "audit")
				_ = audit.CleanupAudits(auditDir, cleanupPolicy)
			}
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
		Config:           cfg,
		ctx:              ctx,
		cancel:           cancel,
	}, nil
}

func (c *Container) Close() {
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