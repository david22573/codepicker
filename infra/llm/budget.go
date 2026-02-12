package llm

import (
	"fmt"
)

// BudgetGuard enforces financial limits on LLM operations.
type BudgetGuard struct {
	tracker        *CostTracker
	maxCost        float64
	alertThreshold float64
}

// NewBudgetGuard initializes a guard with a specific max cost (e.g., $5.00).
func NewBudgetGuard(tracker *CostTracker, maxCost float64) *BudgetGuard {
	return &BudgetGuard{
		tracker:        tracker,
		maxCost:        maxCost,
		alertThreshold: 0.8, // Alert at 80% usage
	}
}

// CheckBeforeCall verifies if the upcoming call is within budget.
func (bg *BudgetGuard) CheckBeforeCall(estimatedTokens int) error {
	stats := bg.tracker.GetMetrics()

	// Check if already over
	if stats.TotalCost >= bg.maxCost {
		return fmt.Errorf("budget exceeded: current cost $%.4f >= limit $%.4f", stats.TotalCost, bg.maxCost)
	}

	// Predict if this call will push us over
	prediction := bg.tracker.PredictCost(estimatedTokens, 500) // Assume 500 completion tokens
	if stats.TotalCost+prediction > bg.maxCost {
		return fmt.Errorf("predicted cost would exceed budget limit")
	}

	return nil
}

// RemainingBudget returns the dollar amount left.
func (bg *BudgetGuard) RemainingBudget() float64 {
	stats := bg.tracker.GetMetrics()
	remaining := bg.maxCost - stats.TotalCost
	if remaining < 0 {
		return 0
	}
	return remaining
}
