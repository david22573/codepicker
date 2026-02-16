package tools

import (
	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/infra/fs"
	"github.com/david22573/codepicker/infra/llm"
	"github.com/david22573/codepicker/infra/shell"
)

// DefaultSet creates the standard list of tools for the agent.
func DefaultSet(
	shadow *fs.ShadowManager,
	sh *shell.Executor,
	root string,
	embedder *llm.EmbeddingClient,
	repo agent.Repository,
) []agent.Tool {
	return []agent.Tool{
		// 1. File Modification (Safe - goes to shadow)
		NewWriteFileTool(shadow),

		// 2. File Reading (Direct - reads from shadow + real fs)
		// Pass shadow manager so agent can read its own pending writes
		NewReadFileTool(root, shadow),
		NewListDirTool(root),

		// 3. Search (Semantic)
		NewSearchTool(embedder, repo),

		// 4. Shell (Sandboxed & Shadow-Aware)
		NewShellTool(sh, shadow),
	}
}
