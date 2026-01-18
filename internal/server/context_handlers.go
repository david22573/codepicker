package server

import (
	"encoding/json"
	"net/http"
)

func (s *AgentServer) handleGetContext(w http.ResponseWriter, r *http.Request) {
	files := s.App.Engine.Memory.List()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"files": files,
		"count": len(files),
	})
}

func (s *AgentServer) handleAddContext(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", 400)
		return
	}

	if err := s.App.Engine.Memory.Add(req.Path); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	s.App.Logger.Info("Manually added to context: " + req.Path)
	json.NewEncoder(w).Encode(map[string]string{"status": "added", "path": req.Path})
}

func (s *AgentServer) handleDeleteContext(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", 400)
		return
	}

	s.App.Engine.Memory.Remove(req.Path)
	s.App.Logger.Info("Manually removed from context: " + req.Path)
	json.NewEncoder(w).Encode(map[string]string{"status": "removed", "path": req.Path})
}
