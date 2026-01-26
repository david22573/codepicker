package errors

import "fmt"

// ErrorCode defines stable error codes for the system
type ErrorCode string

const (
	CodeValidation ErrorCode = "VALIDATION" // User input error
	CodeSystem     ErrorCode = "SYSTEM"     // Internal crash/failure
	CodePolicy     ErrorCode = "POLICY"     // Security block
	CodeLLM        ErrorCode = "LLM"        // AI Provider failure
	CodeNotFound   ErrorCode = "NOT_FOUND"  // Resource missing
)

// DomainError is the standard error type for the domain layer
type DomainError struct {
	Op      string    // Operation where error occurred (e.g., "agent.Run")
	Code    ErrorCode // Machine-readable code
	Message string    // Human-readable message
	Err     error     // Underlying error (optional)
}

func (e *DomainError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %s (cause: %v)", e.Code, e.Op, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s: %s", e.Code, e.Op, e.Message)
}

func (e *DomainError) Unwrap() error { return e.Err }

// Constructors

func NewValidation(op, msg string) *DomainError {
	return &DomainError{Op: op, Code: CodeValidation, Message: msg}
}

func NewSystem(op, msg string, cause error) *DomainError {
	return &DomainError{Op: op, Code: CodeSystem, Message: msg, Err: cause}
}

func NewPolicy(op, msg string) *DomainError {
	return &DomainError{Op: op, Code: CodePolicy, Message: msg}
}

func NewLLM(op string, cause error) *DomainError {
	return &DomainError{Op: op, Code: CodeLLM, Message: "AI provider failure", Err: cause}
}
