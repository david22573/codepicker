package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/david22573/codepicker/pkg/openrouter"
)

func (s *AgentServer) handleAgentTask(w http.ResponseWriter, r *http.Request) {
	// Setup SSE
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
		return
	}

	eventStream := make(chan string)

	// We override the specific approval callback for this request context
	// so we can inject the Notification into THIS stream.
	originalCallback := s.Engine.ApprovalCallback

	// Hook: Intercept approval requests to send them down the SSE pipe
	s.Engine.ApprovalCallback = func(cmdStr, reason string) bool {
		// 1. Send the "Approval Needed" JSON to the client
		// We need to generate the ID here to match the one in WaitForApproval,
		// but WaitForApproval generates it internally.

		// To fix this race/duplication: We actually just use the Server's base WaitForApproval
		// but we need to know the ID it generated.

		// SIMPLIFICATION: We implementation the logic fully here for this request context.
		reqID := fmt.Sprintf("%d", r.Context().Value("req_id_placeholder")) // simplistic
		if reqID == "%!d(string=<nil>)" {
			reqID = "req_" + taskQuery[:3]
		} // fallback

		// Make the channel
		ch := make(chan bool)
		s.approvalLock.Lock()
		s.approvalMap[reqID] = ch
		s.approvalLock.Unlock()

		// Send Event
		jsonMsg, _ := json.Marshal(map[string]interface{}{
			"type":    "approval_req",
			"id":      reqID,
			"command": cmdStr,
			"reason":  reason,
		})
		eventStream <- string(jsonMsg)

		// Wait
		return <-ch
	}

	// Restore callback when done
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
	if exists {
		// Clean up immediately to prevent reuse
		delete(s.approvalMap, req.ID)
	}
	s.approvalLock.Unlock()

	if exists {
		// Send signal to the blocked goroutine
		ch <- req.Approved
		json.NewEncoder(w).Encode(map[string]string{"status": "received"})
	} else {
		http.Error(w, "Request ID not found (timed out or invalid)", 404)
	}
}
