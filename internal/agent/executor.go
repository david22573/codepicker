package agent

import (
	"encoding/json"
	"fmt"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/vfs"
	"github.com/david22573/codepicker/pkg/openrouter"
)

type ToolExecutor struct {
	Memory      *WorkingMemory
	FS          vfs.VirtualFileSystem
	Sentinel    *Sentinel
	Config      *config.ConfigFile
	DryRun      bool   // 3.3: Dry Run Flag
	CurrentTask string // 3.4: Context for Attribution

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
		// Read operations are safe, execute normally even in dry run
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
		// Search is safe
		var args struct {
			Query string `json:"query"`
		}
		if err := json.Unmarshal([]byte(tool.Function.Arguments), &args); err != nil {
			return fmt.Sprintf("Invalid arguments: %v", err)
		}

		if overlay, ok := e.FS.(*vfs.OverlayFS); ok {
			// Access SrcRoot from OverlayFS since search requires file walking logic
			// found in agent.PerformSearch
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

		// 3.3: DRY RUN INTERCEPTION
		if e.DryRun {
			return fmt.Sprintf("[DRY RUN] Would write %d bytes to shadow file: %s", len(args.Content), args.Path)
		}

		path, err := e.FS.WriteFile(args.Path, []byte(args.Content))
		if err != nil {
			return fmt.Sprintf("Error writing shadow file: %v", err)
		}

		// 3.4: RECORD ATTRIBUTION
		if overlay, ok := e.FS.(*vfs.OverlayFS); ok {
			taskName := e.CurrentTask
			if taskName == "" {
				taskName = "Unknown Task"
			}
			// We use "AI Agent" as the generic actor name
			overlay.Shadow.RecordAttribution(args.Path, "AI Agent", taskName)
		}

		return fmt.Sprintf("Changes written to shadow file: %s", path)

	case "run_shell":
		var args struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal([]byte(tool.Function.Arguments), &args); err != nil {
			return fmt.Sprintf("Invalid arguments: %v", err)
		}

		// 3.3: DRY RUN INTERCEPTION
		if e.DryRun {
			return fmt.Sprintf("[DRY RUN] Would execute command: %s", args.Command)
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
