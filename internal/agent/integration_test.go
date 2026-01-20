package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"testing"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/database"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/pkg/openrouter"
)

type MockRoundTripper struct {
	Responses []openrouter.ChatCompletionResponse
	CallCount int
}

func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.CallCount >= len(m.Responses) {
		return nil, fmt.Errorf("unexpected call to LLM")
	}

	resp := m.Responses[m.CallCount]
	m.CallCount++

	body, _ := json.Marshal(resp)
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     make(http.Header),
	}, nil
}

func TestAgentIntegrationFlow(t *testing.T) {

	tmpDir := t.TempDir()
	store, err := database.New(tmpDir)
	if err != nil {
		t.Fatalf("DB init failed: %v", err)
	}
	defer store.Close()

	testFile := "test.txt"
	if err := os.WriteFile(fmt.Sprintf("%s/%s", tmpDir, testFile), []byte("Hello World"), 0644); err != nil {
		t.Fatal(err)
	}

	toolCall := openrouter.ToolCall{
		ID:   "call_1",
		Type: "function",
		Function: openrouter.ToolCallFunction{
			Name:      "read_file",
			Arguments: fmt.Sprintf(`{"path": "%s"}`, testFile),
		},
	}

	finalMsg := openrouter.ChatMessage{
		Role:    "assistant",
		Content: "I have read the file.",
	}

	mockTransport := &MockRoundTripper{
		Responses: []openrouter.ChatCompletionResponse{
			{
				Choices: []openrouter.Choice{{Message: &openrouter.ChatMessage{Role: "assistant", ToolCalls: []openrouter.ToolCall{toolCall}}}},
			},
			{
				Choices: []openrouter.Choice{{Message: &finalMsg}},
			},
		},
	}

	httpClient := &http.Client{Transport: mockTransport}
	client := openrouter.NewClient("fake-key", openrouter.WithHTTPClient(httpClient))

	log := &logger.TestLogger{}
	limits := config.DefaultLimits()
	limits.AgentMaxTurns = 5

	// FIX: Pass nil for config
	engine, err := NewEngine(client, "model", tmpDir, log, limits, store, nil)
	if err != nil {
		t.Fatalf("Engine init failed: %v", err)
	}

	result, err := engine.Run(context.Background(), "Read the test file", nil)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if result != "I have read the file." {
		t.Errorf("Unexpected result: %s", result)
	}

	toolExecFound := false
	for _, l := range log.Logs {
		if contains(l, "Executing Tool: read_file") {
			toolExecFound = true
			break
		}
	}

	if !toolExecFound {
		t.Error("Expected logs to contain tool execution trace")
	}

	files := engine.Memory.List()
	found := slices.Contains(files, testFile)
	if !found {
		t.Error("File was not added to agent memory")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s[0:len(substr)] == substr || contains(s[1:], substr))
}
