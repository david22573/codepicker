package agent

import (
	"github.com/david22573/codepicker/infra/llm"
)

// TurnMemory manages the conversation context using strict token budgeting.
type TurnMemory struct {
	MaxTokens int
}

func NewTurnMemory(maxTokens int) *TurnMemory {
	return &TurnMemory{
		MaxTokens: maxTokens,
	}
}

// Prune reduces the history to fit within the token budget while preserving key context.
// Strategy: Pinned System & Task (First 2 Messages) + Sliding Window (Recent History).
func (m *TurnMemory) Prune(history []llm.Message) []llm.Message {
	// If history is short, no need to prune
	if len(history) <= 3 {
		return history
	}

	// 1. Check if we are actually over budget
	if m.estimateTokens(history) <= m.MaxTokens {
		return history
	}

	// 2. Identify Pinned Messages (System Prompt + User Task)
	// We assume index 0 is System, index 1 is User Task.
	pinned := history[:2]

	// The rest is the "Sliding Window" of conversation
	sliding := history[2:]

	// 3. Prune from the *front* of the sliding window until we fit
	// We want to keep the most recent interactions, so we drop the oldest thoughts/tools.
	for len(sliding) > 0 && m.estimateTokens(append(pinned, sliding...)) > m.MaxTokens {
		sliding = sliding[1:]
	}

	// Reconstruct history
	return append(pinned, sliding...)
}

// estimateTokens provides a rough char-to-token approximation (4 chars ~= 1 token).
// This avoids the overhead of a real tokenizer in the hot loop.
func (m *TurnMemory) estimateTokens(msgs []llm.Message) int {
	chars := 0
	for _, msg := range msgs {
		chars += len(msg.Content)

		// Estimate overhead for tool calls
		if len(msg.ToolCalls) > 0 {
			chars += 200 // JSON structure overhead
			for _, tc := range msg.ToolCalls {
				chars += len(tc.Function.Arguments)
				chars += len(tc.Function.Name)
			}
		}
	}
	return chars / 4
}
