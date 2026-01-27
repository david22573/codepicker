package tools

import (
	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/infra/fs"
	"github.com/david22573/codepicker/infra/shell"
)

// DefaultSet returns the standard set of tools for the coding agent
func DefaultSet(
	fsTools *fs.ShadowManager,
	shellExec *shell.Executor,
	root string,
) []agent.Tool {
	return []agent.Tool{
		NewReadFileTool(fsTools),
		NewWriteFileTool(fsTools),
		NewListFilesTool(root),
		NewSearchTool(root),
		NewShellTool(shellExec),
	}
}
