package ui

// UI defines the standard interaction methods for the CLI.
type UI interface {
	// Simple output
	Info(format string, args ...interface{})
	Success(format string, args ...interface{})
	Warn(format string, args ...interface{})
	Error(format string, args ...interface{})

	// Interactive prompts
	Confirm(question string, defaultYes bool) bool
	Select(question string, items []string) (int, string, error)
	Input(question string, defaultValue string) string

	// Complex rendering
	Table(headers []string, rows [][]string)
}

// Global instance for convenience, though DI is preferred in long run
var Standard UI
