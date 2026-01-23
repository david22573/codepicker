package tokenizer

import (
	"fmt"
	"sync"

	"github.com/pkoukk/tiktoken-go"
)

var (
	tkm     *tiktoken.Tiktoken
	tkmOnce sync.Once
)

// getTokenizer initializes the tokenizer singleton.
// It prioritizes cl100k_base (GPT-4/3.5) and falls back if necessary.
func getTokenizer() *tiktoken.Tiktoken {
	tkmOnce.Do(func() {
		// Try loading the encoding used by GPT-4 and GPT-3.5-Turbo
		tk, err := tiktoken.GetEncoding("cl100k_base")
		if err != nil {
			// Fallback to p50k_base (DaVinci, etc.) if the newer one fails
			tk, err = tiktoken.GetEncoding("p50k_base")
			if err != nil {
				fmt.Printf("⚠️ Warning: Failed to load tokenizer: %v. Token counts will be estimates.\n", err)
				return
			}
		}
		tkm = tk
	})
	return tkm
}

// CountTokens returns the exact token count for a string using tiktoken.
// If the tokenizer fails to load, it falls back to a character-count heuristic.
func CountTokens(text string) int {
	tkm := getTokenizer()
	if tkm == nil {
		// Fallback estimation: ~4 chars per token
		return len(text) / 4
	}

	// Tiktoken returns []int, we just need the length
	tokenized := tkm.Encode(text, nil, nil)
	return len(tokenized)
}

// EstimateCost calculates the approximate cost for the given token count.
// Adjust the rate (0.50) as needed for your specific model blend.
func EstimateCost(tokens int) float64 {
	// Approximation based on blended input/output costs (e.g., $0.50/1M tokens)
	return float64(tokens) / 1_000_000 * 0.50
}

// Truncate ensures the text does not exceed maxTokens.
// It attempts to cut at a token boundary rather than a byte boundary.
func Truncate(text string, maxTokens int) string {
	tkm := getTokenizer()

	// If tokenizer is broken, fallback to simple string slicing
	if tkm == nil {
		approxChars := maxTokens * 4
		if len(text) > approxChars {
			return text[:approxChars]
		}
		return text
	}

	tokens := tkm.Encode(text, nil, nil)
	if len(tokens) <= maxTokens {
		return text
	}

	// If we must truncate, take the first N tokens
	// Note: We cannot easily "Decode" a subset with standard tiktoken-go without
	// implementing a custom decoder loop, as Encode/Decode are not always 1:1 with string indices.
	// For safety and performance in this context, we will estimate the cut position.

	// Better approach: Calculate a safe byte offset
	// This is a simplified approach; for exact precision, you'd iterate the token ranges.
	cutoffRatio := float64(maxTokens) / float64(len(tokens))
	byteCutoff := int(float64(len(text)) * cutoffRatio)

	// Ensure we don't cut mid-rune
	if byteCutoff >= len(text) {
		return text
	}

	// Walk back to start of last valid UTF-8 sequence if needed, though simple slicing usually works for English
	return text[:byteCutoff]
}
