package app

import (
	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/logger"
)

// Context holds the application-wide state and configuration
type Context struct {
	Logger logger.Logger
	Config *config.Config
	Limits *config.Limits

	// Runtime options (set via flags)
	SrcDir      string
	OutPath     string
	Minify      bool
	ShowTokens  bool
	IncludeExts []string
	IgnoreDirs  []string
	Verbose     bool
}

func NewContext() *Context {
	return &Context{
		Limits: config.DefaultLimits(),
		Config: config.NewConfig(),
		Logger: logger.NewStandardLogger(1), // Default info level
	}
}
