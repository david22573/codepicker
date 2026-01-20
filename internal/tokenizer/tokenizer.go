package tokenizer

import (
	"fmt"
	"os"

	"github.com/pkoukk/tiktoken-go"
)

// CountTokens returns the number of tokens in the text string using cl100k_base.
// If the tokenizer fails, it logs a warning to stderr and returns a conservative estimate.
func CountTokens(text string) int {
	tkm, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		// Log explicit warning so the user knows metrics might be off
		fmt.Fprintf(os.Stderr, "⚠️  Tokenizer warning: %v. Using fallback estimation.\n", err)
		// Fallback: 4 chars per token is standard estimation for English text
		return len(text) / 4
	}

	tokenized := tkm.Encode(text, nil, nil)
	return len(tokenized)
}

// EstimateCost calculates USD cost based on DeepSeek/OpenAI pricing
func EstimateCost(tokens int) float64 {
	// $0.50 per 1M tokens (Approximate blend of input/output for cheap models)
	return float64(tokens) / 1_000_000 * 0.50
}
