package tools

import (
	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/shadow"
)

type ToolSet string

const (
	SetReadOnly     ToolSet = "read_only"
	SetStandard     ToolSet = "standard"
	SetAdmin        ToolSet = "admin"
	SetOrchestrator ToolSet = "orchestrator"
)

type Registry struct {
	root      string
	config    *config.ConfigFile
	shadowMgr *shadow.Manager
}

func NewRegistry(root string, cfg *config.ConfigFile, sm *shadow.Manager) *Registry {
	return &Registry{
		root:      root,
		config:    cfg,
		shadowMgr: sm,
	}
}

func (r *Registry) GetImplementation(set ToolSet) []Tool {
	// Base tools available to almost everyone
	read := &ReadFileTool{}
	search := &SearchCodeTool{Root: r.root, Shadow: r.shadowMgr}
	list := &ListFilesTool{Root: r.root}

	// Updated to use GenerateContextTool instead of the undefined ScanPackageTool
	scanner := &GenerateContextTool{
		Root:   r.root,
		Logger: logger.NewStandardLogger(1),
	}

	baseTools := []Tool{read, search, list, scanner}

	switch set {
	case SetReadOnly:
		return baseTools

	case SetStandard:
		// Standard agents can write files but not run shell
		write := &WriteShadowFileTool{Shadow: r.shadowMgr}
		return append(baseTools, write)

	case SetAdmin:
		// Admin agents can do everything including shell
		write := &WriteShadowFileTool{Shadow: r.shadowMgr}
		shell := &RunShellTool{Root: r.root}
		delegate := &DelegateTaskTool{}
		return append(baseTools, write, shell, delegate)

	case SetOrchestrator:
		// Orchestrators focus on planning and delegation
		delegate := &DelegateTaskTool{}
		return append(baseTools, delegate)

	default:
		return baseTools
	}
}
