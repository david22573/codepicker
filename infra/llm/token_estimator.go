package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
)

// TokenEstimator provides a centralized interface for estimating token counts.
type TokenEstimator interface {
	EstimateMessages(msgs []Message) int
	EstimateText(text string) int
}

// NewEstimatorForModel returns a model-specific tokenizer heuristic wrapped in a memoizer.
func NewEstimatorForModel(modelName string) TokenEstimator {
	var base TokenEstimator
	lower := strings.ToLower(modelName)

	if strings.Contains(lower, "gpt") {
		base = NewOpenAIEstimator()
	} else if strings.Contains(lower, "claude") {
		base = NewClaudeEstimator()
	} else {
		base = NewDefaultEstimator()
	}

	return NewMemoizedEstimator(base)
}

// --- Memoized Estimator (Phase 2 Optimization) ---

type MemoizedEstimator struct {
	base  TokenEstimator
	cache sync.Map
}

func NewMemoizedEstimator(base TokenEstimator) *MemoizedEstimator {
	return &MemoizedEstimator{
		base: base,
	}
}

func (m *MemoizedEstimator) EstimateText(text string) int {
	if len(text) == 0 {
		return 0
	}

	hash := hashString(text)
	if val, ok := m.cache.Load(hash); ok {
		return val.(int)
	}

	count := m.base.EstimateText(text)
	m.cache.Store(hash, count)
	return count
}

func (m *MemoizedEstimator) EstimateMessages(msgs []Message) int {
	if len(msgs) == 0 {
		return 0
	}

	hash := hashMessages(msgs)
	if val, ok := m.cache.Load(hash); ok {
		return val.(int)
	}

	count := m.base.EstimateMessages(msgs)
	m.cache.Store(hash, count)
	return count
}

func hashString(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

func hashMessages(msgs []Message) string {
	h := sha256.New()
	// Using JSON marshal for a deterministic representation of the message slice structure
	data, _ := json.Marshal(msgs)
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// --- Default Estimator (Baseline Heuristics) ---

type DefaultEstimator struct {
	charsPerTokenText int
	charsPerTokenCode int
}

func NewDefaultEstimator() *DefaultEstimator {
	return &DefaultEstimator{
		charsPerTokenText: 4,
		charsPerTokenCode: 3,
	}
}

func (e *DefaultEstimator) EstimateText(text string) int {
	if len(text) == 0 {
		return 0
	}
	return len(text) / e.charsPerTokenText
}

func (e *DefaultEstimator) EstimateMessages(msgs []Message) int {
	tokens := 0
	const messageOverhead = 4

	for _, msg := range msgs {
		tokens += messageOverhead
		tokens += len(msg.Content) / e.charsPerTokenCode

		if len(msg.ToolCalls) > 0 {
			tokens += 50
			for _, tc := range msg.ToolCalls {
				tokens += len(tc.Function.Name) / e.charsPerTokenCode
				tokens += len(tc.Function.Arguments) / e.charsPerTokenCode
			}
		}

		if msg.Role == "tool" {
			tokens += 10
		}
	}

	tokens += 3
	return tokens
}

// --- OpenAI Estimator (cl100k_base approximations) ---

type OpenAIEstimator struct {
	base *DefaultEstimator
}

func NewOpenAIEstimator() *OpenAIEstimator {
	return &OpenAIEstimator{
		base: &DefaultEstimator{charsPerTokenText: 4, charsPerTokenCode: 3},
	}
}

func (e *OpenAIEstimator) EstimateText(text string) int {
	return e.base.EstimateText(text)
}

func (e *OpenAIEstimator) EstimateMessages(msgs []Message) int {
	// GPT models have specific overhead per message and name field
	tokens := 0
	for _, msg := range msgs {
		tokens += 3 // <|im_start|>role\n
		tokens += len(msg.Content) / e.base.charsPerTokenCode

		if msg.Name != "" {
			tokens += 1
			tokens += len(msg.Name) / e.base.charsPerTokenCode
		}

		if len(msg.ToolCalls) > 0 {
			tokens += 40 // JSON function call overhead
			for _, tc := range msg.ToolCalls {
				tokens += len(tc.Function.Name) / e.base.charsPerTokenCode
				tokens += len(tc.Function.Arguments) / e.base.charsPerTokenCode
			}
		}
		tokens += 1 // <|im_end|>
	}
	tokens += 3 // <|im_start|>assistant\n
	return tokens
}

// --- Claude Estimator (Anthropic approximations) ---

type ClaudeEstimator struct {
	base *DefaultEstimator
}

func NewClaudeEstimator() *ClaudeEstimator {
	return &ClaudeEstimator{
		// Claude's tokenizer tends to be slightly more efficient on English text
		base: &DefaultEstimator{charsPerTokenText: 5, charsPerTokenCode: 3},
	}
}

func (e *ClaudeEstimator) EstimateText(text string) int {
	return e.base.EstimateText(text)
}

func (e *ClaudeEstimator) EstimateMessages(msgs []Message) int {
	// Anthropic formatting overheads
	tokens := 0
	for _, msg := range msgs {
		tokens += 4
		tokens += len(msg.Content) / e.base.charsPerTokenCode

		if len(msg.ToolCalls) > 0 {
			tokens += 60 // Claude XML tool-use overhead
			for _, tc := range msg.ToolCalls {
				tokens += len(tc.Function.Name) / e.base.charsPerTokenCode
				tokens += len(tc.Function.Arguments) / e.base.charsPerTokenCode
			}
		}
	}
	return tokens
}
