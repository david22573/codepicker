package errors

import "fmt"

type AgentError struct {
	Code      string
	Message   string
	Operation string
	Cause     error
}

func (e *AgentError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %s: %v", e.Code, e.Operation, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s: %s", e.Code, e.Operation, e.Message)
}

func (e *AgentError) Unwrap() error { return e.Cause }

// Constructor functions

func NewToolExecutionError(tool string, cause error) *AgentError {
	return &AgentError{
		Code:      "TOOL_EXEC_FAILED",
		Message:   fmt.Sprintf("Failed to execute tool: %s", tool),
		Operation: "agent.ExecuteTool",
		Cause:     cause,
	}
}

func NewPlanningError(cause error) *AgentError {
	return &AgentError{
		Code:      "PLANNING_FAILED",
		Message:   "AI planning failed to select files",
		Operation: "planner.SelectRelevantFiles",
		Cause:     cause,
	}
}

func NewContextGenerationError(cause error) *AgentError {
	return &AgentError{
		Code:      "CONTEXT_GEN_FAILED",
		Message:   "Failed to generate code context",
		Operation: "contextgen.Generate",
		Cause:     cause,
	}
}
