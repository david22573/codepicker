package logger

import (
	"log"
	"os"
)

// Logger defines the interface for application logging
type Logger interface {
	Info(msg string)
	Warn(msg string)
	Debug(msg string)
	Error(msg string)
}

// StandardLogger implements Logger using the standard log package
type StandardLogger struct {
	logger *log.Logger
	level  int // 0=Error, 1=Info/Warn, 2=Debug
}

// NewStandardLogger creates a logger with the specified verbosity level
func NewStandardLogger(level int) *StandardLogger {
	return &StandardLogger{
		logger: log.New(os.Stderr, "", 0),
		level:  level,
	}
}

func (l *StandardLogger) Info(msg string) {
	if l.level >= 1 {
		l.logger.Printf("ℹ️  %s", msg)
	}
}

func (l *StandardLogger) Warn(msg string) {
	if l.level >= 1 {
		l.logger.Printf("⚠️  %s", msg)
	}
}

func (l *StandardLogger) Debug(msg string) {
	if l.level >= 2 {
		l.logger.Printf("🔧 %s", msg)
	}
}

func (l *StandardLogger) Error(msg string) {
	l.logger.Printf("❌ %s", msg)
}

// NoOpLogger for testing or silent mode
type NoOpLogger struct{}

func (l *NoOpLogger) Info(msg string)  {}
func (l *NoOpLogger) Warn(msg string)  {}
func (l *NoOpLogger) Debug(msg string) {}
func (l *NoOpLogger) Error(msg string) {}

