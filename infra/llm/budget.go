package llm

import (
	"fmt"
)

// BudgetGuard protects against runaway costs by checking limits before LLM calls.
type BudgetGuard struct {
	tracker *CostTracker
	limit   float64
}

func NewBudgetGuard(tracker *CostTracker, limit float64) *BudgetGuard {
	return &BudgetGuard{
		tracker: tracker,
		limit:   limit,
	}
}

// CheckBeforeCall verifies if the upcoming LLM call is within budget.
// It accepts estimated tokens to potentially block massive requests that would blow the budget,
// though currently, it primarily enforces the hard cap on total spent.
func (b *BudgetGuard) CheckBeforeCall(estimatedInputTokens int) error {
	// 0 or negative limit implies "unlimited"
	if b.limit <= 0 {
		return nil
	}

	metrics := b.tracker.GetMetrics()

	// 1. Hard Stop: If we've already spent the money, stop immediately.
	if metrics.TotalCost >= b.limit {
		return fmt.Errorf("budget exceeded (current: $%.4f, limit: $%.4f)", metrics.TotalCost, b.limit)
	}

	// 2. Predictive Check (Optional Safety)
	// If we are extremely close to the limit (e.g., $0.0001 remaining),
	// we could block large requests here.
	// For now, the hard stop is sufficient to prevent "infinite loop" drain.

	return nil
}

// CheckUsage is a helper to verify if we can proceed based on current state.
func (b *BudgetGuard) Remaining() float64 {
	if b.limit <= 0 {
		return 9999.0 // Infinite
	}
	used := b.tracker.GetMetrics().TotalCost
	return b.limit - used
}
