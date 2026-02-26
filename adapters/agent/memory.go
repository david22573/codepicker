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
// It returns the pruned history and the integer count of messages dropped for telemetry.
func (m *TurnMemory) Prune(history []llm.Message) ([]llm.Message, int) {
	if len(history) <= 3 {
		return history, 0
	}

	if m.estimator.EstimateMessages(history) <= m.MaxTokens {
		return history, 0
	}

	var pinned []llm.Message
	slidingStart := 0

	if len(history) >= 2 && history[0].Role == "system" && history[1].Role == "user" {
		pinned = history[:2]
		slidingStart = 2
	} else if len(history) >= 1 && history[0].Role == "system" {
		pinned = history[:1]
		slidingStart = 1
	}

	sliding := history[slidingStart:]
	prunedCount := 0

	for len(sliding) > 0 && m.estimator.EstimateMessages(append(pinned, sliding...)) > m.MaxTokens {
		sliding = sliding[1:]
		prunedCount++
	}

	return append(pinned, sliding...), prunedCount
}

// Estimate provides a robust approximation of token usage via the centralized estimator.
func (m *TurnMemory) Estimate(msgs []llm.Message) int {
	return m.estimator.EstimateMessages(msgs)
}