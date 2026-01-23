package tools

import (
	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/shadow"
	"github.com/david22573/codepicker/pkg/openrouter"
)

type ToolSet string

const (
	SetReadOnly     ToolSet = "read_only"
	SetStandard     ToolSet = "standard"
	SetAdmin        ToolSet = "admin"
	SetOrchestrator ToolSet = "orchestrator"
)

type Registry struct {
	root   string
	config *config.ConfigFile
	shadow *shadow.Manager
}

func NewRegistry(root string, cfg *config.ConfigFile, sm *shadow.Manager) *Registry {
	return &Registry{
		root:   root,
		config: cfg,
		shadow: sm,
	}
}

func (r *Registry) GetDefinitions(set ToolSet) []openrouter.Tool {
	tools := r.GetImplementation(set)
	defs := make([]openrouter.Tool, len(tools))
	for i, t := range tools {
		defs[i] = t.Definition()
	}
	return defs
}

func (r *Registry) GetImplementation(set ToolSet) []Tool {
	read := &ReadFileTool{}
	search := &SearchCodeTool{
		Root:   r.root,
		Shadow: r.shadow,
	}
	write := &WriteShadowFileTool{}
	shell := &RunShellTool{}
	delegate := &DelegateTaskTool{}
	list := &ListFilesTool{Root: r.root}
	skel := &SkeletonizeTool{}

	// NEW: The Scanner Tool
	scanner := &ScanPackageTool{
		Root:   r.root,
		Logger: logger.NewStandardLogger(1),
	}

	var tools []Tool

	// Add 'scanner' to the base set so Architect/Context agents can use it
	base := []Tool{read, search, list, skel, scanner}

	switch set {
	case SetReadOnly:
		tools = base
	case SetStandard:
		tools = append(base, write, delegate)
	case SetAdmin:
		tools = append(base, write, shell, delegate)
	case SetOrchestrator:
		tools = base
	default:
		tools = base
	}

	if (set == SetStandard || set == SetAdmin) && r.config != nil {
		for _, ct := range r.config.CustomTools {
			tools = append(tools, &CustomScriptTool{DefinitionModel: ct})
		}
	}

	return tools
}
