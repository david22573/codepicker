package tokenizer

import (
	"github.com/pkoukk/tiktoken-go"
)

// CountTokens returns the exact number of tokens in the text using GPT-4 encoding.
// It falls back to a simple character count estimation if initialization fails.
func CountTokens(text string) int {
	// "cl100k_base" is the encoding for GPT-4 and GPT-3.5-Turbo
	tkm, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		// Fallback: Estimate 4 characters per token
		return len(text) / 4
	}

	tokenized := tkm.Encode(text, nil, nil)
	return len(tokenized)
}

// EstimateCost calculates the approximate cost in USD (assuming generic blended rate).
// This is purely indicative.
func EstimateCost(tokens int) float64 {
	// $0.50 per 1M tokens (approximate average for input)
	return float64(tokens) / 1_000_000 * 0.50
}
