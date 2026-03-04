package tools

import (
	"context"

	domainAgent "github.com/david22573/codepicker/domain/agent"
)

type ToolRecorder interface {
	RecordTool(name, input, output string, err error)
}

// TracedTool intercepts execution to record a JSON trace.
type TracedTool struct {
	underlying domainAgent.Tool
	recorder   ToolRecorder
}

func NewTracedTool(underlying domainAgent.Tool, recorder ToolRecorder) *TracedTool {
	return &TracedTool{underlying: underlying, recorder: recorder}
}

func (t *TracedTool) Name() string {
	return t.underlying.Name()
}

func (t *TracedTool) Description() string {
	return t.underlying.Description()
}

func (t *TracedTool) Execute(ctx context.Context, args string) (string, error) {
	output, err := t.underlying.Execute(ctx, args)
	t.recorder.RecordTool(t.underlying.Name(), args, output, err)
	return output, err
}

// WrapToolsWithTrace applies the tracing decorator to an entire slice of tools.
func WrapToolsWithTrace(tools []domainAgent.Tool, recorder ToolRecorder) []domainAgent.Tool {
	var traced []domainAgent.Tool
	for _, t := range tools {
		traced = append(traced, NewTracedTool(t, recorder))
	}
	return traced
}
