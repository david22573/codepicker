package llm

import (
	"fmt"
	"sync"
)

// MetricsSnapshot provides a point-in-time view of LLM usage.
type MetricsSnapshot struct {
	TotalTokens      int
	PromptTokens     int
	CompletionTokens int
	TotalCost        float64
	RequestCount     int
}

// CostTracker maintains a thread-safe record of token usage and costs.
type CostTracker struct {
	mu               sync.RWMutex
	totalTokens      int
	promptTokens     int
	completionTokens int
	totalCost        float64
	requestCount     int

	// Per-agent tracking
	agentCosts map[string]float64

	inputCostPer1M  float64
	outputCostPer1M float64
}

// NewCostTracker initializes the tracker with specific model pricing.
func NewCostTracker(inputPrice, outputPrice float64) *CostTracker {
	return &CostTracker{
		inputCostPer1M:  inputPrice,
		outputCostPer1M: outputPrice,
		agentCosts:      make(map[string]float64),
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

// RecordAgentUsage attributes cost to a specific agent ID.
func (ct *CostTracker) RecordAgentUsage(agentID string, prompt, completion int) {
	cost := ct.RecordUsage(prompt, completion)

	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.agentCosts[agentID] += cost
}

// GetMetrics returns a thread-safe snapshot of all metrics.
func (ct *CostTracker) GetMetrics() MetricsSnapshot {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	return MetricsSnapshot{
		TotalTokens:      ct.totalTokens,
		PromptTokens:     ct.promptTokens,
		CompletionTokens: ct.completionTokens,
		TotalCost:        ct.totalCost,
		RequestCount:     ct.requestCount,
	}
}

// GetCostByAgent retrieves the accumulated cost for a specific agent.
func (ct *CostTracker) GetCostByAgent(agentID string) float64 {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.agentCosts[agentID]
}

// PredictCost estimates the cost of a future call.
func (ct *CostTracker) PredictCost(promptTokens, estimatedCompletion int) float64 {
	inputCost := float64(promptTokens) / 1_000_000 * ct.inputCostPer1M
	outputCost := float64(estimatedCompletion) / 1_000_000 * ct.outputCostPer1M
	return inputCost + outputCost
}

// PrintSummary outputs a report to the terminal.
func (ct *CostTracker) PrintSummary() {
	stats := ct.GetMetrics()

	fmt.Println("\n===================================================")
	fmt.Println("💰 SESSION COST SUMMARY")
	fmt.Println("===================================================")
	fmt.Printf("   Requests:     %d\n", stats.RequestCount)
	fmt.Printf("   Total Tokens: %d\n", stats.TotalTokens)
	fmt.Printf("     - Input:    %d\n", stats.PromptTokens)
	fmt.Printf("     - Output:   %d\n", stats.CompletionTokens)
	fmt.Printf("   Est. Cost:    $%.5f\n", stats.TotalCost)
	fmt.Println("===================================================\n")
}
