package agent

import (
	"fmt"
	"github.com/david22573/codepicker/infra/llm"
)

// TurnMemory manages the conversation context using strict token budgeting and adaptive compression.
type TurnMemory struct {
	MaxTokens int
	estimator llm.TokenEstimator
	prefix    *llm.PrefixCache
}

func NewTurnMemory(maxTokens int) *TurnMemory {
	estimator := llm.NewEstimatorForModel("default") // Can be injected for specific model logic
	return &TurnMemory{
		MaxTokens: maxTokens,
		estimator: estimator,
		prefix:    llm.NewPrefixCache(estimator),
	}
}

// Prune reduces the history to fit within the token budget.
// Phase 2: Adaptive Compression replaces dropped turns with a summary node to retain context.
func (m *TurnMemory) Prune(history []llm.Message) ([]llm.Message, int) {
	if len(history) <= 3 {
		return history, 0
	}

	if m.estimator.EstimateMessages(history) <= m.MaxTokens {
		return history, 0
	}

	var pinned []llm.Message
	slidingStart := 0

	// Always pin System and initial User task
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
		// Drop the oldest interaction (Assistant thought + Tool Action pair)
		sliding = sliding[1:]
		prunedCount++
	}

	if prunedCount > 0 {
		// Phase 2: Adaptive Compression - insert a structural memory block
		summaryNode := llm.Message{
			Role: "system",
			Content: fmt.Sprintf("[MEMORY COMPRESSION ACTIVATED: %d older turns have been pruned from context to save tokens. Proceed with the latest observations.]", prunedCount),
		}
		
		var compressedHistory []llm.Message
		compressedHistory = append(compressedHistory, pinned...)
		compressedHistory = append(compressedHistory, summaryNode)
		compressedHistory = append(compressedHistory, sliding...)
		
		return compressedHistory, prunedCount
	}

	return append(pinned, sliding...), 0
}

// Estimate provides a robust approximation of token usage via the centralized estimator.
func (m *TurnMemory) Estimate(msgs []llm.Message) int {
	return m.estimator.EstimateMessages(msgs)
}

// EstimateWithPrefix utilizes the PrefixCache to optimize the cost of static content.
func (m *TurnMemory) EstimateWithPrefix(systemPrompt string, tools []llm.ToolDefinition, dynamicMsgs []llm.Message) int {
	sig := llm.PrefixSignature{
		SystemPrompt: systemPrompt,
		Tools:        tools,
	}
	
	prefixTokens := m.prefix.GetEstimatedTokens(sig)
	dynamicTokens := m.estimator.EstimateMessages(dynamicMsgs)
	
	return prefixTokens + dynamicTokens
}