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
		// Remove time/level from output for cleaner CLI look, unless debugging
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if logLevel == slog.LevelInfo {
				if a.Key == slog.TimeKey || a.Key == slog.LevelKey {
					return slog.Attr{}
				}
			}
			return a
		},
	}

	// Use TextHandler instead of JSON for better human readability in CLI
	handler := slog.NewTextHandler(os.Stderr, opts)

	return &StandardLogger{
		logger: slog.New(handler),
	}
}

func (l *StandardLogger) Info(msg string) {
	// Wrapper to ensure we don't log empty lines
	if msg != "" {
		l.logger.Info(msg)
	}
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

// NoOpLogger for tests
type NoOpLogger struct{}

func (l *NoOpLogger) Info(msg string)  {}
func (l *NoOpLogger) Warn(msg string)  {}
func (l *NoOpLogger) Debug(msg string) {}
func (l *NoOpLogger) Error(msg string) {}

// LogHandler allows capturing logs for testing
type TestLogger struct {
	Logs []string
}

func (l *TestLogger) Info(msg string)  { l.Logs = append(l.Logs, "INFO: "+msg) }
func (l *TestLogger) Warn(msg string)  { l.Logs = append(l.Logs, "WARN: "+msg) }
func (l *TestLogger) Debug(msg string) { l.Logs = append(l.Logs, "DEBUG: "+msg) }
func (l *TestLogger) Error(msg string) { l.Logs = append(l.Logs, "ERROR: "+msg) }
