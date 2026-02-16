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
	var pinned []llm.Message
	slidingStart := 0

	if len(history) >= 2 && history[0].Role == "system" && history[1].Role == "user" {
		pinned = history[:2]
		slidingStart = 2
	} else if len(history) >= 1 && history[0].Role == "system" {
		pinned = history[:1]
		slidingStart = 1
	}

	// The rest is the "Sliding Window" of conversation
	sliding := history[slidingStart:]

	// 3. Prune from the *front* of the sliding window until we fit
	// We want to keep the most recent interactions, so we drop the oldest thoughts/tools.
	for len(sliding) > 0 && m.estimateTokens(append(pinned, sliding...)) > m.MaxTokens {
		sliding = sliding[1:]
	}

	// Reconstruct history
	return append(pinned, sliding...)
}

// estimateTokens provides a robust approximation of token usage.
// OPTIMIZATION: Includes overhead for message structure and JSON tool calls.
func (m *TurnMemory) estimateTokens(msgs []llm.Message) int {
	tokens := 0

	// Base overhead per message (role + formatting)
	// OpenAI standard is ~3-4 tokens per message
	const messageOverhead = 4

	for _, msg := range msgs {
		tokens += messageOverhead

		// Content estimation (approx 3.5 chars per token for code/mixed text)
		tokens += len(msg.Content) / 3

		// Tool Call Overhead
		if len(msg.ToolCalls) > 0 {
			// JSON overhead + array structure
			tokens += 50
			for _, tc := range msg.ToolCalls {
				tokens += len(tc.Function.Name) / 3
				tokens += len(tc.Function.Arguments) / 3
			}
		}

		// Tool Output Overhead (Role: tool)
		if msg.Role == "tool" {
			// Add extra buffer for tool output formatting tags
			tokens += 10
		}
	}

	// Reply buffer (safety margin)
	tokens += 3

	return tokens
}
