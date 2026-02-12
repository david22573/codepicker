package mocks

import (
	"context"
	"sync"

	domainCtx "github.com/david22573/codepicker/domain/context"
)

// CallRecord captures the details of a single LLM interaction for verification in tests.
type CallRecord struct {
	SystemPrompt string
	UserMessage  string
	Timestamp    int64
}

// MockLLMClient is a robust, thread-safe mock for the agent.LLMClient interface.
type MockLLMClient struct {
	// Configuration: Queued responses and errors
	responses []string
	errors    []error

	// State: Current position in the queue
	callIndex int
	mu        sync.Mutex

	// Verification: History of calls made
	Calls []CallRecord
}

// NewMockLLM creates a client pre-loaded with a sequence of responses.
func NewMockLLM(responses ...string) *MockLLMClient {
	return &MockLLMClient{
		responses: responses,
		errors:    make([]error, len(responses)), // Initialize with nil errors
		Calls:     make([]CallRecord, 0),
	}
}

// Chat implements the basic agent.LLMClient interface.
func (m *MockLLMClient) Chat(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 1. Record the call
	m.Calls = append(m.Calls, CallRecord{
		SystemPrompt: systemPrompt,
		UserMessage:  userMessage,
	})

	// 2. Check bounds
	if m.callIndex >= len(m.responses) {
		return "Final Answer: [MOCK] Sequence exhausted.", nil
	}

	// 3. Check for forced error at this turn
	if err := m.errors[m.callIndex]; err != nil {
		m.callIndex++
		return "", err
	}

	// 4. Return queued response
	resp := m.responses[m.callIndex]
	m.callIndex++
	return resp, nil
}

// ChatWithUsage implements the extended cost-tracking interface.
// It returns fixed token usage to ensure tests are deterministic.
func (m *MockLLMClient) ChatWithUsage(ctx context.Context, sys, user string) (string, domainCtx.TokenUsage, error) {
	resp, err := m.Chat(ctx, sys, user)

	// Mock usage data
	usage := domainCtx.TokenUsage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}

	return resp, usage, err
}

// --- Test Helper Methods ---

// QueueResponse adds a new response to the end of the queue.
func (m *MockLLMClient) QueueResponse(response string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses = append(m.responses, response)
	m.errors = append(m.errors, nil)
}

// QueueError sets a specific turn to fail with the given error.
// index is 0-based.
func (m *MockLLMClient) QueueErrorAt(index int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Expand slice if index is out of bounds
	for len(m.errors) <= index {
		m.errors = append(m.errors, nil)
		// We must also pad responses to keep alignment, though they won't be used if error triggers
		m.responses = append(m.responses, "")
	}

	m.errors[index] = err
}

// GetLastCall returns the most recent interaction. Useful for assertions.
func (m *MockLLMClient) GetLastCall() CallRecord {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.Calls) == 0 {
		return CallRecord{}
	}
	return m.Calls[len(m.Calls)-1]
}

// Reset clears the call history and resets the index (but keeps the scripted responses).
func (m *MockLLMClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callIndex = 0
	m.Calls = make([]CallRecord, 0)
}
