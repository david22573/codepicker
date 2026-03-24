package app

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/david22573/codepicker/adapters/agent"
	ctxAdapters "github.com/david22573/codepicker/adapters/context"
	"github.com/david22573/codepicker/adapters/tools"
	"github.com/david22573/codepicker/adapters/verifier"
	"github.com/david22573/codepicker/domain/config"
	domainCtx "github.com/david22573/codepicker/domain/context"
	"github.com/david22573/codepicker/domain/event"
	"github.com/david22573/codepicker/infra/audit"
	infraConfig "github.com/david22573/codepicker/infra/config"
	"github.com/david22573/codepicker/infra/fs"
	"github.com/david22573/codepicker/infra/git"
	"github.com/david22573/codepicker/infra/indexer"
	"github.com/david22573/codepicker/infra/llm"
	"github.com/david22573/codepicker/infra/logging"
	"github.com/david22573/codepicker/infra/metrics"
	"github.com/david22573/codepicker/infra/shell"
	"github.com/david22573/codepicker/infra/storage"
	"github.com/david22573/codepicker/infra/trace"
	"github.com/david22573/codepicker/runtime"
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
	EmbedClient      *llm.EmbeddingClient
	EventBus         *event.DataBus
	Logger           *logging.Logger
	CostTracker      *llm.CostTracker
	Config           *config.AppConfig
	TraceRecorder    *trace.Recorder
	CostObserver     *agent.CostObserver
	RepoMapper       *indexer.RepoMapper

	wg     sync.WaitGroup
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

	metrics.SetRegistry(metrics.NewPrometheusBackend())

	cfgPath := filepath.Join(rootDir, "codepicker.yaml")
	loader := infraConfig.NewLoader(cfgPath)
	cfg, err := loader.Load()
	if err != nil {
		logger.Warn("Failed to load configuration file, using defaults", zap.Error(err))
		cfg = config.DefaultConfig()
	}

	if cfg.Environment == "production" {
		runtime.Global.Mode = runtime.ModeProduction
	} else if cfg.Environment == "hardened-ci" || ciMode {
		runtime.Global.Mode = runtime.ModeHardenedCI
	} else {
		runtime.Global.Mode = runtime.ModeDevelopment
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

	recordTraceDir := os.Getenv("CODEPICKER_RECORD_TRACE")
	replayTracePath := os.Getenv("CODEPICKER_REPLAY_TRACE")

	var activeLLM llm.Provider = llmClient

	shellExec := shell.NewExecutor(30*time.Second, 5000, dryRun, rootDir)
	gitClient := git.NewClient(rootDir, dryRun)
	autoCommit := os.Getenv("CODEPICKER_NO_AUTOCOMMIT") != "1"
	activeTools := tools.DefaultSet(shadowMgr, shellExec, rootDir, embedClient, repo, gitClient, activeLLM, autoCommit)

	var activeRecorder *trace.Recorder

	if replayTracePath != "" {
		logger.Info("Starting in DETERMINISTIC REPLAY MODE", zap.String("trace", replayTracePath))
		replayState, err := trace.LoadReplayState(replayTracePath)
		if err != nil {
			return nil, fmt.Errorf("failed to load replay trace: %w", err)
		}
		activeLLM = llm.NewReplayAdapter(replayState)
		activeTools = tools.GenerateReplayTools(activeTools, replayState)
	} else if recordTraceDir != "" {
		sessionID := fmt.Sprintf("run_%d", time.Now().Unix())
		logger.Info("Starting in TRACE RECORDING MODE", zap.String("dir", recordTraceDir), zap.String("session", sessionID))
		activeRecorder = trace.NewRecorder(sessionID, recordTraceDir)
		activeLLM = llm.NewTraceAdapter(activeLLM, activeRecorder)
		activeTools = tools.WrapToolsWithTrace(activeTools, activeRecorder)
	}

	mapper := indexer.NewRepoMapper()

	// Phase 4: Wire the repo_map_cache logic so cold starts don't re-index everything
	var db indexer.DB
	if rawDB, ok := interface{}(repo).(indexer.DB); ok {
		db = rawDB
	} else if dbProvider, ok := interface{}(repo).(interface{ DB() *sql.DB }); ok {
		db = dbProvider.DB()
	}

	if db != nil {
		if err := mapper.LoadCache(context.Background(), db); err != nil {
			logger.Warn("Failed to load repo map cache", zap.Error(err))
		}
	}

	_, planner, executor, auditor, explainer, twoPass, ctxBuilder, err := NewAgentStack(AgentStackOpts{
		Config:       cfg,
		LLMClient:    activeLLM,
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
		Mapper:       mapper,
	}, activeTools)

	if err != nil {
		return nil, err
	}

	slicer := indexer.NewCodeSlicer()
	indexManager := indexer.NewIndexManager(slicer, repo, embedClient)

	// Ensure we don't silently drop sync errors
	err = indexManager.SyncRepoMap(context.Background(), rootDir, mapper)
	if err != nil {
		logger.Warn("Failed to sync repo map, agent context may be incomplete", zap.Error(err))
	} else if db != nil {
		go func() {
			if saveErr := mapper.SaveCache(context.Background(), db); saveErr != nil {
				logger.Warn("Failed to save repo map cache", zap.Error(saveErr))
			}
		}()
	}

	primer := ctxAdapters.NewProjectPrimer(rootDir, mapper, false)
	verifierPipeline := verifier.NewPipeline(rootDir)

	ctx, cancel := context.WithCancel(context.Background())

	costObserver := agent.NewCostObserver(repo, logger)
	costObserver.Start(ctx, eventBus.Subscribe())

	c := &Container{
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
		EmbedClient:      embedClient,
		EventBus:         eventBus,
		Logger:           logger,
		CostTracker:      costTracker,
		Config:           cfg,
		TraceRecorder:    activeRecorder,
		CostObserver:     costObserver,
		RepoMapper:       mapper,
		ctx:              ctx,
		cancel:           cancel,
	}

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
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

	return c, nil
}

func (c *Container) Close() {
	if c.cancel != nil {
		c.cancel()
	}
	if c.CostObserver != nil {
		c.CostObserver.Stop()
	}

	c.wg.Wait()

	if c.TraceRecorder != nil {
		c.TraceRecorder.Finish()
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
