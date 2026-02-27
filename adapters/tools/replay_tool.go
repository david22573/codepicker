package tools

import (
	"context"

	domainAgent "github.com/david22573/codepicker/domain/agent"
)

type ToolReplayer interface {
	NextTool(name string) (string, error)
}

// ReplayTool mimics a real tool but serves its deterministic output from a transcript.
type ReplayTool struct {
	name        string
	description string
	replayer    ToolReplayer
}

func NewReplayTool(name, description string, replayer ToolReplayer) *ReplayTool {
	return &ReplayTool{name: name, description: description, replayer: replayer}
}

func (r *ReplayTool) Name() string {
	return r.name
}

func (r *ReplayTool) Description() string {
	return r.description
}

func (r *ReplayTool) Execute(ctx context.Context, args string) (string, error) {
	return r.replayer.NextTool(r.name)
}

// GenerateReplayTools replaces an active toolset with inert replay versions.
func GenerateReplayTools(originalTools []domainAgent.Tool, replayer ToolReplayer) []domainAgent.Tool {
	var replayTools []domainAgent.Tool
	for _, t := range originalTools {
		replayTools = append(replayTools, NewReplayTool(t.Name(), t.Description(), replayer))
	}
	return replayTools
}