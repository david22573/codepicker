package llm

import (
	"fmt"
	"sync"
)

// BudgetGuard protects against runaway costs by checking limits before LLM calls.
type BudgetGuard struct {
	tracker  *CostTracker
	limit    float64
	reserved float64 // Tracks funds reserved but not yet billed
	mu       sync.Mutex
}

// NewBudgetGuard initializes the guard with a tracker and a hard limit.
func NewBudgetGuard(tracker *CostTracker, limit float64) *BudgetGuard {
	return &BudgetGuard{
		tracker: tracker,
		limit:   limit,
	}
}

// Reserve attempts to allocate budget for an estimated cost atomically.
// Call this BEFORE the LLM request.
func (b *BudgetGuard) Reserve(estimatedCost float64) error {
	// 0 or negative limit implies "unlimited"
	if b.limit <= 0 {
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	metrics := b.tracker.GetMetrics()

	currentTotal := metrics.TotalCost + b.reserved

	if currentTotal+estimatedCost > b.limit {
		return fmt.Errorf("budget exceeded: current usage $%.4f + reserved $%.4f exceeds limit $%.4f",
			metrics.TotalCost, b.reserved, b.limit)
	}

	b.reserved += estimatedCost
	return nil
}

// Commit releases the reservation after the actual cost has been recorded.
// Call this AFTER the LLM request (usually in a defer).
func (b *BudgetGuard) Commit(reservedAmount float64) {
	if b.limit <= 0 {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// Release the reservation
	b.reserved -= reservedAmount

	// Safety clamp to prevent negative reservation due to rounding errors
	if b.reserved < 0 {
		b.reserved = 0
	}
}

// CheckBeforeCall maintains backward compatibility but is less safe than Reserve/Commit.
func (b *BudgetGuard) CheckBeforeCall(estimatedInputTokens int) error {
	// Simple estimation for backward compatibility (approx $0.001 per call)
	return b.Reserve(0.001)
}

// Remaining calculates how much budget is left, accounting for reservations.
func (b *BudgetGuard) Remaining() float64 {
	if b.limit <= 0 {
		return 9999.0 // Infinite
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	used := b.tracker.GetMetrics().TotalCost
	return b.limit - (used + b.reserved)
}
