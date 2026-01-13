package server

import (
	"encoding/json"
	"errors"
	"net/http"

	cpErrors "github.com/david22573/codepicker/internal/errors"
)

// WriteError inspects the error type and writes the appropriate JSON response
func (s *AgentServer) WriteError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")

	// Check if it's our custom structured error
	var cpErr *cpErrors.CodePickerError
	if errors.As(err, &cpErr) {
		status := http.StatusInternalServerError

		// Map codes to HTTP statuses
		switch cpErr.Code {
		case "VALIDATION_ERROR":
			status = http.StatusBadRequest
		case "SCAN_FAILED":
			status = http.StatusInternalServerError
		case "NOT_FOUND":
			status = http.StatusNotFound
		}

		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(cpErr)
		return
	}

	// Fallback for standard Go errors
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"code":    "UNKNOWN_ERROR",
		"message": err.Error(),
	})
}
