package llm

// TokenEstimator provides a centralized interface for estimating token counts.
type TokenEstimator interface {
	EstimateMessages(msgs []Message) int
	EstimateText(text string) int
}

// DefaultEstimator provides a heuristic-based token estimation strategy.
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
			tokens += 50 // JSON overhead
			for _, tc := range msg.ToolCalls {
				tokens += len(tc.Function.Name) / e.charsPerTokenCode
				tokens += len(tc.Function.Arguments) / e.charsPerTokenCode
			}
		}

		if msg.Role == "tool" {
			tokens += 10
		}
	}
	
	tokens += 3 // Reply buffer
	return tokens
}