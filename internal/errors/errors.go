package errors

import (
	"fmt"
)

type ErrorType string

const (
	ErrorTypeInternal ErrorType = "INTERNAL"
	ErrorTypeUser     ErrorType = "USER"
	ErrorTypeLLM      ErrorType = "LLM"
)

type AppError struct {
	Type    ErrorType
	Code    string
	Message string
	Err     error
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %s: %v", e.Type, e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s: %s", e.Type, e.Code, e.Message)
}

func (e *AppError) Unwrap() error {
	return e.Err
}

// NewInternalError creates a new internal error (code bugs, system failures)
func NewInternalError(code string, err error) error {
	return &AppError{
		Type:    ErrorTypeInternal,
		Code:    code,
		Message: "Internal system error",
		Err:     err,
	}
}

// NewUserError creates a new error caused by invalid user input
func NewUserError(code string, message string) error {
	return &AppError{
		Type:    ErrorTypeUser,
		Code:    code,
		Message: message,
	}
}

// NewLLMError creates a new error related to LLM interactions
func NewLLMError(code string, err error) error {
	return &AppError{
		Type:    ErrorTypeLLM,
		Code:    code,
		Message: "LLM provider error",
		Err:     err,
	}
}
