package tools

import (
	"context"

	"github.com/david22573/codepicker/pkg/openrouter"
)

// RuntimeContext holds the dependencies required for tools to execute.
// It uses interfaces to decouple from specific internal packages (like agent or vfs).
type RuntimeContext struct {
	// FS provides file system access (Read/Write)
	FS FileSystem

	// Memory allows tools to add files to the agent's working memory
	Memory MemoryManager

	// Sentinel provides security checks for shell commands
	Sentinel SecuritySentinel

	// Config provides access to global settings (like allowed extensions)
	Config ConfigProvider

	// Worker is used for the 'delegate_task' tool
	Worker WorkerAgent
}

// Tool defines the standard interface for all agent capabilities.
type Tool interface {
	// Name returns the function name (e.g. "read_file")
	Name() string

	// Description returns the help text for the LLM
	Description() string

	// Definition returns the OpenRouter/OpenAI compatible tool schema
	Definition() openrouter.Tool

	// Execute runs the tool with the provided JSON arguments
	Execute(ctx context.Context, argsJSON string, runtime *RuntimeContext) (string, error)
}

// --- Dependency Interfaces ---

type FileSystem interface {
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, content []byte) (string, error)
}

type MemoryManager interface {
	Add(path string) error
}

type SecuritySentinel interface {
	// CheckCommand returns: needsApproval, reason, binary, args
	CheckCommand(cmdStr string) (bool, string, string, []string)
	Execute(binary string, args []string) (string, error)
}

type ConfigProvider interface {
	IsExtensionAllowed(ext string) bool
	IsDirIgnored(dir string) bool
}

type WorkerAgent interface {
	Run(ctx context.Context, instruction string, files []string) (string, error)
}
