package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/david22573/codepicker/pkg/openrouter"
)

func (s *AgentServer) handleAgentTask(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	taskQuery := r.URL.Query().Get("q")
	if taskQuery == "" {
		fmt.Fprintf(w, "data: {\"type\": \"error\", \"msg\": \"Query empty\"}\n\n")
		flusher.Flush()
		return
	}

	eventStream := make(chan string)
	originalCallback := s.Engine.ApprovalCallback

	reqID := fmt.Sprintf("%s", r.Context().Value("request_id"))
	if reqID == "%!s(<nil>)" || reqID == "" {
		reqID = "req_" + taskQuery[:3]
	}

	// Fix: Use buffered channel and handle context cancellation to prevent deadlocks
	s.Engine.ApprovalCallback = func(cmdStr, reason string) bool {
		ch := make(chan bool, 1)

		s.approvalLock.Lock()
		s.approvalMap[reqID] = ch
		s.approvalLock.Unlock()

		defer func() {
			s.approvalLock.Lock()
			delete(s.approvalMap, reqID)
			s.approvalLock.Unlock()
		}()

		jsonMsg, _ := json.Marshal(map[string]interface{}{
			"type":    "approval_req",
			"id":      reqID,
			"command": cmdStr,
			"reason":  reason,
		})

		// Send request to client with timeout/context check
		select {
		case eventStream <- string(jsonMsg):
		case <-r.Context().Done():
			return false
		case <-time.After(5 * time.Second): // Timeout if stream is blocked
			s.Logger.Warn("Timed out writing approval request to stream")
			return false
		}

		// Wait for response
		select {
		case approved := <-ch:
			return approved
		case <-r.Context().Done():
			return false
		case <-time.After(60 * time.Second):
			s.Logger.Warn(fmt.Sprintf("Approval timed out for %s", reqID))
			return false
		}
	}

	defer func() { s.Engine.ApprovalCallback = originalCallback }()

	go func() {
		defer close(eventStream)

		updateFn := func(msg openrouter.ChatMessage) {
			jsonMsg, _ := json.Marshal(map[string]interface{}{
				"type":    "thought",
				"role":    msg.Role,
				"content": msg.Content,
			})
			eventStream <- string(jsonMsg)
		}

		result, err := s.Engine.Run(r.Context(), taskQuery, updateFn)

		if err != nil {
			errJSON, _ := json.Marshal(map[string]string{"type": "error", "msg": err.Error()})
			eventStream <- string(errJSON)
		} else {
			doneJSON, _ := json.Marshal(map[string]string{"type": "done", "result": result})
			eventStream <- string(doneJSON)
		}
	}()

	for event := range eventStream {
		fmt.Fprintf(w, "data: %s\n\n", event)
		flusher.Flush()
	}
}

func (s *AgentServer) handleApprovalResponse(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	var req struct {
		ID       string `json:"id"`
		Approved bool   `json:"approved"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", 400)
		return
	}

	s.approvalLock.Lock()
	ch, exists := s.approvalMap[req.ID]
	s.approvalLock.Unlock()

	if exists {
		// Non-blocking send in case the receiver has already given up
		select {
		case ch <- req.Approved:
			json.NewEncoder(w).Encode(map[string]string{"status": "received"})
		default:
			http.Error(w, "Approval receiver not listening", 408)
		}
	} else {
		http.Error(w, "Request ID not found (timed out or invalid)", 404)
	}
}
