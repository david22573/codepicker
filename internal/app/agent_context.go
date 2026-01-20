package app

import (
	"context"
	"fmt"
	"os"

	"github.com/david22573/codepicker/internal/agent"
	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/constants"
	"github.com/david22573/codepicker/internal/database"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/paths"
	"github.com/david22573/codepicker/internal/policy"
	"github.com/david22573/codepicker/pkg/openrouter"
)

type RunMode string

const (
	ModeInteractive RunMode = "interactive"
	ModeBatch       RunMode = "batch"
	ModeServer      RunMode = "server"
)

type AgentContext struct {
	Ctx    context.Context
	Cancel context.CancelFunc

	Config *config.ConfigFile
	Limits *config.Limits
	Logger logger.Logger
	Store  *database.Store
	Engine *agent.Engine

	SrcDir string
}

type ContextOptions struct {
	SrcDir   string
	LogLevel int
	Mode     RunMode
	Policy   policy.ExecutionPolicy
	Task     string // Optional, for context awareness
}

func NewAgentContext(ctx context.Context, opts ContextOptions) (*AgentContext, error) {
	log := logger.NewStandardLogger(opts.LogLevel)

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENROUTER_API_KEY environment variable is not set")
	}

	// This is the SINGLE point where config is loaded
	cfgFile, err := config.GetOrLoadConfig("")
	if err != nil {
		log.Warn(fmt.Sprintf("Failed to load config: %v", err))
	}

	absSrc, err := paths.Sanitize(opts.SrcDir)
	if err != nil {
		return nil, fmt.Errorf("invalid source directory: %w", err)
	}

	store, err := database.New(".codepicker")
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	client := openrouter.NewClient(apiKey)
	limits := config.DefaultLimits()

	model := constants.DefaultModel
	if cfgFile != nil && cfgFile.AI.Model != "" {
		model = cfgFile.AI.Model
	}

	eng, err := agent.NewEngine(
		client,
		model,
		absSrc,
		log,
		limits,
		store,
		cfgFile, // Pass explicit config
	)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("engine initialization failed: %w", err)
	}

	eng.SetPolicy(opts.Policy)

	ctx, cancel := context.WithCancel(ctx)

	return &AgentContext{
		Ctx:    ctx,
		Cancel: cancel,
		Config: cfgFile,
		Limits: limits,
		Logger: log,
		Store:  store,
		Engine: eng,
		SrcDir: absSrc,
	}, nil
}

func (ac *AgentContext) Close() {
	if ac.Store != nil {
		ac.Store.Close()
	}
	ac.Cancel()
}
