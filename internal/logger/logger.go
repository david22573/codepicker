package logger

import (
	"log/slog"
	"os"
)

type Logger interface {
	Info(msg string)
	Warn(msg string)
	Debug(msg string)
	Error(msg string)
}

type StandardLogger struct {
	logger *slog.Logger
}

func NewStandardLogger(level int) *StandardLogger {
	// Map integer level to slog.Level
	// 0=Error (default), 1=Info, 2=Debug
	var logLevel slog.Level
	switch level {
	case 2:
		logLevel = slog.LevelDebug
	case 1:
		logLevel = slog.LevelInfo
	default:
		logLevel = slog.LevelError
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	// Create a JSON handler for structured output
	handler := slog.NewJSONHandler(os.Stderr, opts)

	return &StandardLogger{
		logger: slog.New(handler),
	}
}

func (l *StandardLogger) Info(msg string) {
	l.logger.Info(msg)
}

func (l *StandardLogger) Warn(msg string) {
	l.logger.Warn(msg)
}

func (l *StandardLogger) Debug(msg string) {
	l.logger.Debug(msg)
}

func (l *StandardLogger) Error(msg string) {
	l.logger.Error(msg)
}

type NoOpLogger struct{}

func (l *NoOpLogger) Info(msg string)  {}
func (l *NoOpLogger) Warn(msg string)  {}
func (l *NoOpLogger) Debug(msg string) {}
func (l *NoOpLogger) Error(msg string) {}
