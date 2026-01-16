package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/david22573/codepicker/internal/agent"
	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/pkg/openrouter"
)

func setupTestServer(t *testing.T) *AgentServer {
	tmpDir := t.TempDir()
	client := openrouter.NewClient("fake-key")
	engine, err := agent.NewEngine(client, "model", tmpDir, &logger.NoOpLogger{}, config.DefaultLimits())
	if err != nil {
		t.Fatalf("Engine init failed: %v", err)
	}

	return New("8080", engine, &logger.NoOpLogger{})
}

func TestHandleHealth(t *testing.T) {
	srv := setupTestServer(t)

	req, _ := http.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()

	handler := http.HandlerFunc(srv.handleHealth)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	expected := `{"status":"ok"}`
	if strings.TrimSpace(rr.Body.String()) != expected {
		t.Errorf("handler returned unexpected body: got %v want %v", rr.Body.String(), expected)
	}
}

func TestHandleGetContext(t *testing.T) {
	srv := setupTestServer(t)

	// Inject a fake file into memory
	srv.Engine.Memory.Files["test.go"] = agent.FileSnapshot{
		Path:    "test.go",
		Content: "package test",
		Tokens:  10,
	}

	req, _ := http.NewRequest("GET", "/agent/context", nil)
	rr := httptest.NewRecorder()

	handler := http.HandlerFunc(srv.handleGetContext)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("returned wrong status: got %v", rr.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &resp)

	files, ok := resp["files"].([]interface{})
	if !ok || len(files) != 1 {
		t.Errorf("Expected 1 file in context, got %v", resp["files"])
	}
}
