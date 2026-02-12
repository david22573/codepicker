package logging

import (
	"context"

	"go.uber.org/zap"
)

// TraceContext links log events across turns for Kimi K2.5's long-running tasks.
type TraceContext struct {
	ExecutionID string
	AgentName   string
	Turn        int
}

// WithTrace enriches the logger with structured tracing fields.
func (l *Logger) WithTrace(tc TraceContext) *Logger {
	return &Logger{
		zap: l.zap.With(
			zap.String("execution_id", tc.ExecutionID),
			zap.String("agent_name", tc.AgentName),
			zap.Int("turn", tc.Turn),
		),
	}
}

// ContextWithTrace returns a context enriched with trace info using the unified ExecutionIDKey.
func ContextWithTrace(ctx context.Context, tc TraceContext) context.Context {
	return context.WithValue(ctx, ExecutionIDKey, tc.ExecutionID)
}

// GetExecutionID extracts the ID from context if it exists.
func GetExecutionID(ctx context.Context) string {
	if id, ok := ctx.Value(ExecutionIDKey).(string); ok {
		return id
	}
	return ""
}
