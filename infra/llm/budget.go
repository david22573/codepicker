package llm

import (
	"fmt"
	"math"
	"sync"
)

// BudgetGuard protects against runaway costs by checking limits before LLM calls.
type BudgetGuard struct {
	tracker  *CostTracker
	limit    float64
	reserved float64 
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
func (b *BudgetGuard) Reserve(estimatedCost float64) error {
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
func (b *BudgetGuard) Commit(reservedAmount float64) {
	if b.limit <= 0 {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.reserved -= reservedAmount

	if b.reserved < 0 {
		b.reserved = 0
	}
}

// CheckBeforeCall maintains backward compatibility but is less safe than Reserve/Commit.
func (b *BudgetGuard) CheckBeforeCall(estimatedInputTokens int) error {
	return b.Reserve(0.001)
}

// Remaining calculates how much budget is left, accounting for reservations.
func (b *BudgetGuard) Remaining() float64 {
	if b.limit <= 0 {
		return math.MaxFloat64 // Infinite
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	used := b.tracker.GetMetrics().TotalCost
	return b.limit - (used + b.reserved)
}