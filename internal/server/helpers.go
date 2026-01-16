package server

import (
	"encoding/json"
	"errors"
	"net/http"

	cpErrors "github.com/david22573/codepicker/internal/errors"
)

func (s *AgentServer) WriteError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")

	// Check for standard CodePickerError
	var cpErr *cpErrors.CodePickerError
	if errors.As(err, &cpErr) {
		status := http.StatusInternalServerError

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

	// Check for AgentError (New in Phase 1)
	var agentErr *cpErrors.AgentError
	if errors.As(err, &agentErr) {
		w.WriteHeader(http.StatusUnprocessableEntity) // 422 for agent logic failures
		_ = json.NewEncoder(w).Encode(map[string]string{
			"code":      agentErr.Code,
			"message":   agentErr.Message,
			"operation": agentErr.Operation,
		})
		return
	}

	// Fallback for unknown errors
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"code":    "UNKNOWN_ERROR",
		"message": err.Error(),
	})
}
