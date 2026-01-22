package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/david22573/codepicker/pkg/openrouter"
)

type TaskRequest struct {
	Task string `json:"task"`
}

type TaskResponse struct {
	Result string `json:"result"`
	Status string `json:"status"`
}

type ApprovalRequestPayload struct {
	RequestID string `json:"request_id"`
	Approved  bool   `json:"approved"`
}

type ContextRequest struct {
	Path string `json:"path"`
}

// handleHealth returns simple status
func (s *AgentServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"mode":   string(s.App.Engine.Enforcer.Policy.Mode),
		"uptime": "running",
	})
}

// handleAgentTask executes a task synchronously (for now)
func (s *AgentServer) handleAgentTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req TaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if req.Task == "" {
		http.Error(w, "Task cannot be empty", http.StatusBadRequest)
		return
	}

	// Define a simple callback to log or stream updates (optional: support SSE later)
	updateFn := func(msg openrouter.ChatMessage) {
		// Log progress for server-side visibility
		if msg.Role == "tool" {
			s.App.Logger.Debug(fmt.Sprintf("Tool Output: %s", msg.ToolCallID))
		}
	}

	result, err := s.App.Engine.Run(r.Context(), req.Task, updateFn)
	if err != nil {
		s.App.Logger.Error(fmt.Sprintf("Task failed: %v", err))
		http.Error(w, fmt.Sprintf("Agent error: %v", err), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(TaskResponse{
		Status: "success",
		Result: result,
	})
}

// handleApprovalResponse allows an external user/UI to approve a pending tool action
func (s *AgentServer) handleApprovalResponse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ApprovalRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	s.approvalLock.Lock()
	ch, exists := s.approvalMap[req.RequestID]
	s.approvalLock.Unlock()

	if !exists {
		http.Error(w, "Approval request ID not found or expired", http.StatusNotFound)
		return
	}

	// Send decision to the waiting goroutine
	// Non-blocking send to avoid hanging if the waiter gave up
	select {
	case ch <- req.Approved:
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "received"})
	default:
		http.Error(w, "Request timed out on server side", http.StatusGone)
	}
}

func (s *AgentServer) handleGetContext(w http.ResponseWriter, r *http.Request) {
	// FIX: Use List() instead of ListFiles() to match WorkingMemory interface
	files := s.App.Engine.Memory.List()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"files": files,
		"count": len(files),
	})
}

func (s *AgentServer) handleAddContext(w http.ResponseWriter, r *http.Request) {
	var req ContextRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if err := s.App.Engine.Memory.Add(req.Path); err != nil {
		http.Error(w, fmt.Sprintf("Failed to add file: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "added", "path": req.Path})
}

func (s *AgentServer) handleDeleteContext(w http.ResponseWriter, r *http.Request) {
	var req ContextRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	s.App.Engine.Memory.Remove(req.Path)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "removed", "path": req.Path})
}
