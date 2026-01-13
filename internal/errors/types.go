package errors

import (
	"fmt"
)

// CodePickerError is a structured error designed for JSON responses
type CodePickerError struct {
	Code      string                 `json:"code"`
	Message   string                 `json:"message"`
	Operation string                 `json:"operation"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Err       error                  `json:"-"` // Internal error, do not serialize
}

func (e *CodePickerError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Operation, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Operation, e.Message)
}

func (e *CodePickerError) Unwrap() error {
	return e.Err
}

// Helper Constructors

func New(code, msg, op string, err error) *CodePickerError {
	return &CodePickerError{
		Code:      code,
		Message:   msg,
		Operation: op,
		Err:       err,
	}
}

func NewValidationError(field, msg, value string) *CodePickerError {
	return &CodePickerError{
		Code:      "VALIDATION_ERROR",
		Message:   fmt.Sprintf("%s: %s", field, msg),
		Operation: "validation",
		Metadata:  map[string]interface{}{"value": value},
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

func NewInternalError(op string, err error) *CodePickerError {
	return &CodePickerError{
		Code:      "INTERNAL_ERROR",
		Message:   "An unexpected internal error occurred",
		Operation: op,
		Err:       err,
	}
}
