package openrouter_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/david22573/codepicker/pkg/openrouter"
)

func TestCreateChatCompletion(t *testing.T) {
	// Mock Server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify Request Headers
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("Missing Auth Header")
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("Wrong path: %s", r.URL.Path)
		}

		// Verify Request Body
		var req openrouter.ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("Failed to decode body: %v", err)
		}
		if req.Model != "test-model" {
			t.Errorf("Wrong model: %s", req.Model)
		}
		if len(req.Messages) == 0 {
			t.Errorf("No messages sent")
		}

		// Mock Stream Response
		// This simulates a Server-Sent Events (SSE) stream
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		data1 := `{"choices": [{"delta": {"content": "Hello"}}], "id": "1", "model": "test"}`
		data2 := `{"choices": [{"delta": {"content": " World"}}], "usage": {"total_tokens": 10}}`

		w.Write([]byte("data: " + data1 + "\n\n"))
		w.Write([]byte("data: " + data2 + "\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer mockServer.Close()

	// Client
	client := openrouter.NewClient(
		"test-key",
		openrouter.WithBaseURL(mockServer.URL),
	)

	req := openrouter.ChatCompletionRequest{
		Model: "test-model",
		Messages: []openrouter.ChatMessage{
			{Role: "user", Content: "Hi"},
		},
	}

	resp, err := client.CreateChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateChatCompletion failed: %v", err)
	}

	expected := "Hello World"
	if resp.Choices[0].Message.Content != expected {
		t.Errorf("Content mismatch: got %q, want %q", resp.Choices[0].Message.Content, expected)
	}

	if resp.Usage.TotalTokens != 10 {
		t.Errorf("Usage mismatch: got %d, want 10", resp.Usage.TotalTokens)
	}
}

func TestListModels(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": [{"id": "gpt-4", "name": "GPT-4"}]}`))
	}))
	defer mockServer.Close()

	client := openrouter.NewClient("key", openrouter.WithBaseURL(mockServer.URL))

	resp, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}

	if len(resp.Data) != 1 || resp.Data[0].ID != "gpt-4" {
		t.Errorf("Unexpected model data: %+v", resp.Data)
	}
}
