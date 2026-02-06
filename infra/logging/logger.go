package logging

import (
	"context"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger struct {
	zap *zap.Logger
}

type contextKey string

const (
	requestIDKey   contextKey = "request_id"
	executionIDKey contextKey = "execution_id"
)

// NewLogger initializes the logging system.
// env: "production" (JSON, Info level) or "development" (Console, Debug level)
func NewLogger(env string) (*Logger, error) {
	var config zap.Config

	if env == "production" {
		config = zap.NewProductionConfig()
		config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		config.OutputPaths = []string{"stdout", ".codepicker/app.log"}
	} else {
		config = zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		config.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("15:04:05")
		// Shorten caller path to just package/file
		config.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
	}

	// Build with stacktrace only for errors
	logger, err := config.Build(
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)
	if err != nil {
		return nil, err
	}

	return &Logger{zap: logger}, nil
}

// WithContext enriches the logger with trace IDs from the context
func (l *Logger) WithContext(ctx context.Context) *Logger {
	fields := []zap.Field{}

	// Extract standard tracing keys if they exist
	if reqID, ok := ctx.Value(requestIDKey).(string); ok {
		fields = append(fields, zap.String("request_id", reqID))
	}

	if execID, ok := ctx.Value(executionIDKey).(string); ok {
		fields = append(fields, zap.String("execution_id", execID))
	}

	if len(fields) > 0 {
		return &Logger{zap: l.zap.With(fields...)}
	}
	return l
}

// Core logging methods
func (l *Logger) Info(msg string, fields ...zap.Field) {
	l.zap.Info(msg, fields...)
}

func (l *Logger) Error(msg string, fields ...zap.Field) {
	l.zap.Error(msg, fields...)
}

func (l *Logger) Warn(msg string, fields ...zap.Field) {
	l.zap.Warn(msg, fields...)
}

func (l *Logger) Debug(msg string, fields ...zap.Field) {
	l.zap.Debug(msg, fields...)
}

func (l *Logger) Fatal(msg string, fields ...zap.Field) {
	l.zap.Fatal(msg, fields...)
}

// Sync flushes any buffered logs (call this on exit)
func (l *Logger) Sync() error {
	return l.zap.Sync()
}

// --- Domain Specific Helpers ---

// LogAgentAction logs high-level agent decisions
func (l *Logger) LogAgentAction(agentName, action string, metadata map[string]interface{}) {
	fields := []zap.Field{
		zap.String("component", "agent"),
		zap.String("agent_name", agentName),
		zap.String("action", action),
	}

	for k, v := range metadata {
		fields = append(fields, zap.Any(k, v))
	}

	l.Info("Agent Action", fields...)
}

// LogToolExecution records the result and duration of a tool call
func (l *Logger) LogToolExecution(toolName, args string, duration time.Duration, err error) {
	fields := []zap.Field{
		zap.String("component", "tool"),
		zap.String("tool", toolName),
		zap.String("args", args),
		zap.Duration("duration", duration),
	}

	if err != nil {
		fields = append(fields, zap.Error(err))
		l.Error("Tool Execution Failed", fields...)
	} else {
		l.Info("Tool Execution Success", fields...)
	}
}

// LogLLMCall tracks token usage and latency for cost monitoring
func (l *Logger) LogLLMCall(model string, duration time.Duration, promptTokens, completionTokens int, err error) {
	fields := []zap.Field{
		zap.String("component", "llm"),
		zap.String("model", model),
		zap.Duration("duration", duration),
		zap.Int("tokens_prompt", promptTokens),
		zap.Int("tokens_completion", completionTokens),
		zap.Int("tokens_total", promptTokens+completionTokens),
	}

	if err != nil {
		fields = append(fields, zap.Error(err))
		l.Error("LLM Request Failed", fields...)
	} else {
		l.Info("LLM Request Completed", fields...)
	}
}
