package agent

import (
	"encoding/json"
	"fmt"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/vfs"
	"github.com/david22573/codepicker/pkg/openrouter"
)

type ToolExecutor struct {
	Memory   *WorkingMemory
	FS       vfs.VirtualFileSystem
	Sentinel *Sentinel
	Config   *config.ConfigFile

	OnApproval func(command, reason string) bool
}

func NewToolExecutor(mem *WorkingMemory, fs vfs.VirtualFileSystem, s *Sentinel, cfg *config.ConfigFile) *ToolExecutor {
	return &ToolExecutor{
		Memory:     mem,
		FS:         fs,
		Sentinel:   s,
		Config:     cfg,
		OnApproval: func(c, r string) bool { return true },
	}
}

func (e *ToolExecutor) Execute(tool openrouter.ToolCall) string {
	switch tool.Function.Name {
	case "read_file":
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal([]byte(tool.Function.Arguments), &args); err != nil {
			return fmt.Sprintf("Invalid arguments: %v", err)
		}
		if err := e.Memory.Add(args.Path); err != nil {
			return fmt.Sprintf("Error reading '%s': %v", args.Path, err)
		}
		return fmt.Sprintf("✓ File '%s' loaded into context", args.Path)

	case "search_code":
		var args struct {
			Query string `json:"query"`
		}
		if err := json.Unmarshal([]byte(tool.Function.Arguments), &args); err != nil {
			return fmt.Sprintf("Invalid arguments: %v", err)
		}

		// Note: Search still needs raw access to SrcRoot/Shadow logic to traverse directories,
		// but since we are refactoring logic step-by-step, we'll keep the direct call to PerformSearch
		// (which uses filepath.Walk) for now, as VFS is currently file-level access only.
		// Ideally, VFS should eventually support Walk/Glob.
		// For now, we assume Memory.FS is an OverlayFS which has the SrcRoot available if needed,
		// but PerformSearch takes a root string path.

		// Accessing srcRoot via type assertion if needed, or keeping it strictly separated.
		// Since PerformSearch is a utility function taking a string path, we rely on the engine's root.
		// However, ToolExecutor doesn't hold SrcRoot directly anymore.
		// We'll assume for this pass that we can't easily fix Search without expanding VFS,
		// so we might need to cast or pass SrcRoot in NewToolExecutor if we want to keep identical behavior.
		// BUT: PerformSearch is imported from agent package (tools.go).
		// Let's assume for this specific refactor we grab the root from the OverlayFS if possible,
		// or update NewToolExecutor to accept srcRoot strictly for search.

		// To keep it clean: We will attempt to use the Memory's underlying concept or just re-inject srcRoot.
		// Let's check where PerformSearch is called. It uses e.Memory.SrcRoot which we deleted.
		// We need to pass SrcRoot to ToolExecutor explicitly or via FS.

		if overlay, ok := e.FS.(*vfs.OverlayFS); ok {
			results, err := PerformSearch(overlay.SrcRoot, args.Query)
			if err != nil {
				return fmt.Sprintf("Search error: %v", err)
			}
			return results
		}
		return "Search unavailable (VFS does not support path resolution)"

	case "write_shadow_file":
		var args struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal([]byte(tool.Function.Arguments), &args); err != nil {
			return fmt.Sprintf("Invalid arguments: %v", err)
		}
		path, err := e.FS.WriteFile(args.Path, []byte(args.Content))
		if err != nil {
			return fmt.Sprintf("Error writing shadow file: %v", err)
		}
		return fmt.Sprintf("Changes written to shadow file: %s", path)

	case "run_shell":
		var args struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal([]byte(tool.Function.Arguments), &args); err != nil {
			return fmt.Sprintf("Invalid arguments: %v", err)
		}

		needsApproval, reason, binary, cmdArgs := e.Sentinel.CheckCommand(args.Command)
		if needsApproval {
			if e.OnApproval != nil && !e.OnApproval(args.Command, reason) {
				return "Command denied by user."
			}
		}

		out, err := e.Sentinel.Execute(binary, cmdArgs)
		if err != nil {
			return fmt.Sprintf("Command failed: %v\nOutput: %s", err, out)
		}
		return out

	default:
		out, err := ExecuteCustomTool(tool.Function.Name, tool.Function.Arguments, e.Config)
		if err != nil {
			return fmt.Sprintf("Tool execution error: %v", err)
		}
		return out
	}
}
