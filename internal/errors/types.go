package errors

import (
	"fmt"
)

// CodePickerError is the standard error type for the application
type CodePickerError struct {
	Code      string                 `json:"code"`
	Message   string                 `json:"message"`
	Operation string                 `json:"operation"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Err       error                  `json:"-"` // Internal error, do not serialize
}

func (e *CodePickerError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %s: %v", e.Code, e.Operation, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s: %s", e.Code, e.Operation, e.Message)
}

func (e *CodePickerError) Unwrap() error {
	return e.Err
}

// Generic Constructors

func New(code, msg, op string, err error) *CodePickerError {
	return &CodePickerError{
		Code:      code,
		Message:   msg,
		Operation: op,
		Err:       err,
	}
}

func NewInternalError(op string, err error) *CodePickerError {
	return &CodePickerError{
		Code:      "INTERNAL_ERROR",
		Message:   "An unexpected internal error occurred",
		Operation: op,
		Err:       err,
	}
}

// Domain Specific Constructors

func NewValidationError(field, msg, value string) *CodePickerError {
	return &CodePickerError{
		Code:      "VALIDATION_ERROR",
		Message:   fmt.Sprintf("Invalid %s: %s", field, msg),
		Operation: "validation",
		Metadata:  map[string]interface{}{"value": value},
	}
}

func NewConfigError(msg string, err error) *CodePickerError {
	return &CodePickerError{
		Code:      "CONFIG_ERROR",
		Message:   msg,
		Operation: "config.Load",
		Err:       err,
	}
}

func NewLLMError(op string, err error) *CodePickerError {
	return &CodePickerError{
		Code:      "LLM_ERROR",
		Message:   "AI Provider request failed",
		Operation: op,
		Err:       err,
	}
}

func NewScanError(path string, err error) *CodePickerError {
	return &CodePickerError{
		Code:      "SCAN_FAILED",
		Message:   "Failed to scan file or directory",
		Operation: "scanner.Scan",
		Metadata:  map[string]interface{}{"path": path},
		Err:       err,
	}
}
