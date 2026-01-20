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

	// FIX: Save the original callback from Enforcer
	originalCallback := s.App.Engine.Enforcer.OnApproval

	reqID := fmt.Sprintf("%s", r.Context().Value("request_id"))
	if reqID == "%!s(<nil>)" || reqID == "" {
		reqID = "req_" + taskQuery[:3]
	}

	// FIX: Set the new callback on Enforcer
	s.App.Engine.Enforcer.OnApproval = func(cmdStr, reason string) bool {

		select {
		case <-r.Context().Done():
			s.App.Logger.Warn("Client disconnected before approval request")
			return false
		default:
		}

		ch := make(chan bool, 1)

		s.approvalLock.Lock()
		s.approvalMap[reqID] = ch
		s.approvalLock.Unlock()

		defer func() {
			s.approvalLock.Lock()
			delete(s.approvalMap, reqID)
			s.approvalLock.Unlock()
			close(ch)
		}()

		jsonMsg, _ := json.Marshal(map[string]interface{}{
			"type":    "approval_req",
			"id":      reqID,
			"command": cmdStr,
			"reason":  reason,
		})

		select {
		case eventStream <- string(jsonMsg):
		case <-r.Context().Done():
			s.App.Logger.Warn("Client disconnected during approval dispatch")
			return false
		case <-time.After(5 * time.Second):
			s.App.Logger.Warn("Timed out writing approval request to stream")
			return false
		}

		select {
		case approved := <-ch:
			return approved
		case <-r.Context().Done():
			s.App.Logger.Warn(fmt.Sprintf("Client disconnected while waiting for approval on %s", reqID))
			return false
		case <-time.After(60 * time.Second):
			s.App.Logger.Warn(fmt.Sprintf("Approval timed out for %s", reqID))
			return false
		}
	}

	// FIX: Restore original callback on Enforcer
	defer func() { s.App.Engine.Enforcer.OnApproval = originalCallback }()

	go func() {
		defer close(eventStream)

		updateFn := func(msg openrouter.ChatMessage) {
			select {
			case <-r.Context().Done():
				return
			default:
				jsonMsg, _ := json.Marshal(map[string]interface{}{
					"type":    "thought",
					"role":    msg.Role,
					"content": msg.Content,
				})
				eventStream <- string(jsonMsg)
			}
		}

		result, err := s.App.Engine.Run(r.Context(), taskQuery, updateFn)

		if err != nil {
			if r.Context().Err() != nil {
				return
			}
			errJSON, _ := json.Marshal(map[string]string{"type": "error", "msg": err.Error()})
			eventStream <- string(errJSON)
		} else {
			doneJSON, _ := json.Marshal(map[string]string{"type": "done", "result": result})
			eventStream <- string(doneJSON)
		}
	}()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-eventStream:
			if !ok {
				return
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", event); err != nil {
				return
			}
			flusher.Flush()
		}
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
