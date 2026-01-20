package tools

import (
	"github.com/david22573/codepicker/internal/config"
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
}

// Update constructor to take config
func NewRegistry(root string, cfg *config.ConfigFile) *Registry {
	return &Registry{
		root:   root,
		config: cfg,
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
	search := &SearchCodeTool{Root: r.root}
	write := &WriteShadowFileTool{}
	shell := &RunShellTool{}
	delegate := &DelegateTaskTool{}

	var tools []Tool

	// Base tools
	base := []Tool{read, search}

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

	// Add Custom Tools (only for Standard and Admin sets)
	if (set == SetStandard || set == SetAdmin) && r.config != nil {
		for _, ct := range r.config.CustomTools {
			tools = append(tools, &CustomScriptTool{DefinitionModel: ct})
		}
	}

	return tools
}
