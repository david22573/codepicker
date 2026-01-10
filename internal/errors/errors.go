package errors

import (
	"fmt"
	"path/filepath"
	"strings"
)

type ScannerError struct {
	Path string
	Err  error
}

func (e *ScannerError) Error() string {
	return fmt.Sprintf("scan error at %s: %v", e.Path, e.Err)
}

func (e *ScannerError) Unwrap() error {
	return e.Err
}

type APIError struct {
	Status   int
	Body     string
	Model    string
	Endpoint string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API error (status %d) on %s: %s [model: %s]",
		e.Status, e.Endpoint, e.Body, e.Model)
}

type ValidationError struct {
	Field   string
	Message string
	Value   string
}

func (e *ValidationError) Error() string {
	if e.Value != "" {
		return fmt.Sprintf("validation failed for %s: %s (got: %s)",
			e.Field, e.Message, e.Value)
	}
	return fmt.Sprintf("validation failed for %s: %s", e.Field, e.Message)
}

type WriterError struct {
	Action string
	Path   string
	Err    error
}

func (e *WriterError) Error() string {
	return fmt.Sprintf("writer error during %s on %s: %v",
		e.Action, e.Path, e.Err)
}

func (e *WriterError) Unwrap() error {
	return e.Err
}

func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	switch e := err.(type) {
	case *APIError:
		return e.Status >= 500 || e.Status == 429
	case *ScannerError:
		return strings.Contains(e.Err.Error(), "temporary") ||
			strings.Contains(e.Err.Error(), "timeout")
	default:
		return false
	}
}

type PathError struct {
	Op   string // "stat", "read", "write", "mkdir", "abs"
	Path string
	Err  error
}

func (e *PathError) Error() string {
	return fmt.Sprintf("path %s failed for %s: %v", e.Op, e.Path, e.Err)
}

func (e *PathError) Unwrap() error {
	return e.Err
}

func NewScannerError(path string, err error) error {
	rel, relErr := filepath.Rel(".", path)
	if relErr == nil && !strings.HasPrefix(rel, "..") {
		path = rel
	}
	return &ScannerError{Path: path, Err: err}
}

func NewAPIError(status int, body, model, endpoint string) error {
	if len(body) > 200 {
		body = body[:200] + "..."
	}
	return &APIError{
		Status:   status,
		Body:     body,
		Model:    model,
		Endpoint: endpoint,
	}
}

func NewPathError(op, path string, err error) error {
	return &PathError{
		Op:   op,
		Path: path,
		Err:  err,
	}
}

