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

// RunMode dictates the overarching behavior of the application
type RunMode string

const (
	ModeInteractive RunMode = "interactive"
	ModeBatch       RunMode = "batch"
	ModeServer      RunMode = "server"
)

// AgentContext serves as the unified runtime for the application.
// It holds the dependencies required to run an agent loop.
type AgentContext struct {
	Ctx    context.Context
	Cancel context.CancelFunc

	Config *config.ConfigFile
	Limits *config.Limits
	Logger logger.Logger
	Store  *database.Store
	Engine *agent.Engine

	// Runtime Paths
	SrcDir string
}

type ContextOptions struct {
	SrcDir   string
	LogLevel int
	Mode     RunMode
	Policy   policy.ExecutionPolicy
	Task     string // Optional, for context awareness
}

// NewAgentContext initializes the full runtime environment.
func NewAgentContext(ctx context.Context, opts ContextOptions) (*AgentContext, error) {
	// 1. Setup Logging
	log := logger.NewStandardLogger(opts.LogLevel)

	// 2. Load Config & Env
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENROUTER_API_KEY environment variable is not set")
	}
	cfgFile, _ := config.LoadConfigFile("")

	// 3. Resolve Paths
	absSrc, err := paths.Sanitize(opts.SrcDir)
	if err != nil {
		return nil, fmt.Errorf("invalid source directory: %w", err)
	}

	// 4. Initialize Database (Centralized)
	store, err := database.New(".codepicker")
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// 5. Initialize Engine Components
	client := openrouter.NewClient(apiKey)
	limits := config.DefaultLimits()

	// Apply Model overrides from Config if present
	model := constants.DefaultModel
	if cfgFile != nil && cfgFile.AI.Model != "" {
		model = cfgFile.AI.Model
	}

	// 6. Bootstrap Engine
	eng, err := agent.NewEngine(
		client,
		model,
		absSrc,
		log,
		limits,
		store,
	)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("engine initialization failed: %w", err)
	}

	// 7. Apply Policy
	eng.SetPolicy(opts.Policy)

	// 8. Create Context with Cancellation
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

// Close ensures resources like the DB are shut down cleanly.
func (ac *AgentContext) Close() {
	if ac.Store != nil {
		ac.Store.Close()
	}
	ac.Cancel()
}
