package tools

import (
	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/infra/fs"
	"github.com/david22573/codepicker/infra/git"
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
	gitClient *git.Client,
	llmClient agent.LLMClient,
	autoCommit bool,
) []agent.Tool {
	return []agent.Tool{
		// 1. File Modification (Safe - goes to shadow)
		NewWriteFileTool(shadow),
		NewEditFileTool(root, shadow, gitClient, llmClient, autoCommit),

		// 2. File Reading & Exploration
		NewReadFileTool(root, shadow),
		NewListDirTool(root),
		NewSkeletonTool(root),
		NewDefinitionSearchTool(root),

		// 3. Search (Semantic)
		NewSearchTool(embedder, repo),

		// 4. Git & Shell
		NewGitDiffTool(root),
		NewShellTool(sh, shadow),
	}
}
