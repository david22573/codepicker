package llm

import (
	"fmt"
	"sync"
)

// CostTracker maintains a thread-safe record of token usage and costs.
type CostTracker struct {
	mu               sync.Mutex
	totalTokens      int
	promptTokens     int
	completionTokens int
	totalCost        float64
	requestCount     int

	// Pricing per 1 Million tokens (Adjust based on your OpenRouter model)
	// Defaulting to Kimi/Moonshot estimates (approximate)
	inputCostPer1M  float64
	outputCostPer1M float64
}

// NewCostTracker initializes the tracker with specific model pricing.
func NewCostTracker(inputPrice, outputPrice float64) *CostTracker {
	return &CostTracker{
		inputCostPer1M:  inputPrice,
		outputCostPer1M: outputPrice,
	}
}

// RecordUsage updates the metrics for a single LLM interaction.
func (ct *CostTracker) RecordUsage(prompt, completion int) float64 {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	inputCost := float64(prompt) / 1_000_000 * ct.inputCostPer1M
	outputCost := float64(completion) / 1_000_000 * ct.outputCostPer1M
	reqCost := inputCost + outputCost

	ct.promptTokens += prompt
	ct.completionTokens += completion
	ct.totalTokens += (prompt + completion)
	ct.totalCost += reqCost
	ct.requestCount++

	return reqCost
}

// GetStats returns the current totals.
func (ct *CostTracker) GetStats() (total int, cost float64, requests int) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	return ct.totalTokens, ct.totalCost, ct.requestCount
}

// PrintSummary outputs a user-friendly cost report to the terminal.
func (ct *CostTracker) PrintSummary() {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	fmt.Println("\n===================================================")
	fmt.Println("💰 SESSION COST SUMMARY")
	fmt.Println("===================================================")
	fmt.Printf("   Requests:     %d\n", ct.requestCount)
	fmt.Printf("   Total Tokens: %d\n", ct.totalTokens)
	fmt.Printf("     - Input:    %d\n", ct.promptTokens)
	fmt.Printf("     - Output:   %d\n", ct.completionTokens)
	fmt.Printf("   Est. Cost:    $%.5f\n", ct.totalCost)
	fmt.Println("===================================================\n")
}
