package tracking

import (
	"fmt"
	"sync"
	"time"
)

type CostTracker struct {
	mu           sync.RWMutex
	totalCost    float64
	requestCount int
	dailyLimit   float64
	currentDate  string
}

func NewCostTracker(dailyLimit float64) *CostTracker {
	return &CostTracker{
		dailyLimit:  dailyLimit,
		currentDate: time.Now().Format("2006-01-02"),
	}
}

func (ct *CostTracker) RecordRequest(promptTokens, completionTokens int, model string) error {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	// Reset if it's a new day
	today := time.Now().Format("2006-01-02")
	if today != ct.currentDate {
		ct.totalCost = 0
		ct.requestCount = 0
		ct.currentDate = today
	}

	// Calculate cost (Estimates based on typical high-end model pricing)
	// In production, you might want to fetch exact model pricing from OpenRouter
	cost := calculateCost(promptTokens, completionTokens, model)

	if ct.totalCost+cost > ct.dailyLimit {
		return fmt.Errorf("daily cost limit exceeded: $%.4f / $%.2f",
			ct.totalCost+cost, ct.dailyLimit)
	}

	ct.totalCost += cost
	ct.requestCount++
	return nil
}

func (ct *CostTracker) GetStats() (float64, int) {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.totalCost, ct.requestCount
}

func calculateCost(promptTokens, completionTokens int, model string) float64 {
	// Default to a safe upper bound estimate (approx $5/1M input, $15/1M output)
	// Real costs depend heavily on the specific model used.
	inputRate := 5.0
	outputRate := 15.0

	inputCost := float64(promptTokens) / 1_000_000 * inputRate
	outputCost := float64(completionTokens) / 1_000_000 * outputRate
	return inputCost + outputCost
}
