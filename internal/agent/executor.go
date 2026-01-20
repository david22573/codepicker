package agent

import (
	"encoding/json"
	"fmt"
	"strings"

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
		var args struct {
			Path      string `json:"path"`
			StartLine int    `json:"start_line,omitempty"`
			EndLine   int    `json:"end_line,omitempty"`
		}
		if err := json.Unmarshal([]byte(tool.Function.Arguments), &args); err != nil {
			return fmt.Sprintf("Invalid arguments: %v", err)
		}

		// 1. Read full file content from VFS
		contentBytes, err := e.FS.ReadFile(args.Path)
		if err != nil {
			return fmt.Sprintf("Error reading '%s': %v", args.Path, err)
		}

		// 2. Handle partial read if lines are specified
		if args.StartLine > 0 || args.EndLine > 0 {
			lines := strings.Split(string(contentBytes), "\n")

			// Convert 1-based start line to 0-based index
			start := args.StartLine - 1
			if start < 0 {
				start = 0
			}

			// Handle end line (inclusive)
			end := args.EndLine
			if end == 0 || end > len(lines) {
				end = len(lines)
			}

			// Validation
			if start >= len(lines) {
				return fmt.Sprintf("Start line %d is beyond file length (%d lines)", args.StartLine, len(lines))
			}
			if start > end {
				return fmt.Sprintf("Invalid range: start %d > end %d", args.StartLine, args.EndLine)
			}

			// Slice and return strictly the requested content
			subset := strings.Join(lines[start:end], "\n")
			return fmt.Sprintf("--- FILE: %s (Lines %d-%d) ---\n%s", args.Path, args.StartLine, args.EndLine, subset)
		}

		// 3. Default: Load full file into context memory
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

		if e.DryRun {
			return fmt.Sprintf("[DRY RUN] Would write %d bytes to shadow file: %s", len(args.Content), args.Path)
		}

		path, err := e.FS.WriteFile(args.Path, []byte(args.Content))
		if err != nil {
			return fmt.Sprintf("Error writing shadow file: %v", err)
		}

		if overlay, ok := e.FS.(*vfs.OverlayFS); ok {
			taskName := e.CurrentTask
			if taskName == "" {
				taskName = "Unknown Task"
			}
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
