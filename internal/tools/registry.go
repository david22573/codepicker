package tools

import "github.com/david22573/codepicker/pkg/openrouter"

type ToolSet string

const (
	SetReadOnly     ToolSet = "read_only"
	SetStandard     ToolSet = "standard"
	SetAdmin        ToolSet = "admin"
	SetOrchestrator ToolSet = "orchestrator"
)

// Registry helps organize and retrieve tools for different agent types.
type Registry struct {
	root string
}

func NewRegistry(root string) *Registry {
	return &Registry{root: root}
}

// GetTools returns a slice of OpenRouter tool definitions for a specific profile.
func (r *Registry) GetDefinitions(set ToolSet) []openrouter.Tool {
	tools := r.GetImplementation(set)
	defs := make([]openrouter.Tool, len(tools))
	for i, t := range tools {
		defs[i] = t.Definition()
	}
	return defs
}

// GetImplementation returns the concrete Tool interfaces.
func (r *Registry) GetImplementation(set ToolSet) []Tool {
	read := &ReadFileTool{}
	search := &SearchCodeTool{Root: r.root}
	write := &WriteShadowFileTool{}
	shell := &RunShellTool{} // Note: OnApproval needs to be set by the caller if interactive
	delegate := &DelegateTaskTool{}

	base := []Tool{read, search}

	switch set {
	case SetReadOnly:
		return base
	case SetStandard:
		return append(base, write, delegate)
	case SetAdmin:
		return append(base, write, shell, delegate)
	case SetOrchestrator:
		return base // Orchestrators usually just read/plan
	default:
		return base
	}
}
