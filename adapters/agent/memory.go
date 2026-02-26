package agent

import (
	"github.com/david22573/codepicker/infra/llm"
)

// TurnMemory manages the conversation context using strict token budgeting.
type TurnMemory struct {
	MaxTokens int
	estimator llm.TokenEstimator
}

func NewTurnMemory(maxTokens int) *TurnMemory {
	return &TurnMemory{
		MaxTokens: maxTokens,
		estimator: llm.NewDefaultEstimator(),
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
	if m.estimator.EstimateMessages(history) <= m.MaxTokens {
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
	for len(sliding) > 0 && m.estimator.EstimateMessages(append(pinned, sliding...)) > m.MaxTokens {
		sliding = sliding[1:]
	}

	// Reconstruct history
	return append(pinned, sliding...)
}

// Estimate provides a robust approximation of token usage via the centralized estimator.
func (m *TurnMemory) Estimate(msgs []llm.Message) int {
	return m.estimator.EstimateMessages(msgs)
}