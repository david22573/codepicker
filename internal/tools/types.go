package tools

import (
	"context"

	"github.com/david22573/codepicker/pkg/openrouter"
)

type RuntimeContext struct {
	FS FileSystem

	Memory MemoryManager

	Sentinel SecuritySentinel

	Config ConfigProvider

	Worker WorkerAgent
}

type Tool interface {
	Name() string

	Description() string

	Definition() openrouter.Tool

	// [3.3] Capability-Driven Security
	Capabilities() []Capability

	Execute(ctx context.Context, argsJSON string, runtime *RuntimeContext) (string, error)
}

type FileSystem interface {
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, content []byte) (string, error)
}

type MemoryManager interface {
	Add(path string) error
	Snapshot() (interface{}, error) // Generic interface to avoid circular deps
	Restore(snap interface{}) error
}

type SecuritySentinel interface {
	CheckCommand(cmdStr string) (bool, string, string, []string)
	ClassifyCommand(cmdStr string) string
	Execute(binary string, args []string) (string, error)
}

type ConfigProvider interface {
	IsExtensionAllowed(ext string) bool
	IsDirIgnored(dir string) bool
}

type WorkerAgent interface {
	Run(ctx context.Context, instruction string, files []string) (string, error)
}
