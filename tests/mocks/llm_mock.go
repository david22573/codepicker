package mocks

import (
	"context"
)

type MockLLMClient struct {
	Responses []string
	Index     int
}

func (m *MockLLMClient) Chat(ctx context.Context, sys, user string) (string, error) {
	if m.Index >= len(m.Responses) {
		return "Final Answer: Done.", nil
	}
	resp := m.Responses[m.Index]
	m.Index++
	return resp, nil
}
