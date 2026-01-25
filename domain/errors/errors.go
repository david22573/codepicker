package errors

import "fmt"

type DomainError struct {
	Op      string
	Code    string
	Message string
	Err     error
}

func (e *DomainError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %s (cause: %v)", e.Code, e.Op, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s: %s", e.Code, e.Op, e.Message)
}

func (e *DomainError) Unwrap() error { return e.Err }

// Common constructors
func NewValidation(op, msg string) error {
	return &DomainError{Op: op, Code: "VALIDATION", Message: msg}
}

func NewSystem(op, msg string, cause error) error {
	return &DomainError{Op: op, Code: "SYSTEM", Message: msg, Err: cause}
}
