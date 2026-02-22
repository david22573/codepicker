package unit

import (
	"testing"

	"github.com/david22573/codepicker/infra/llm"
)

func TestBudgetGuard(t *testing.T) {
	// Setup tracker with dummy pricing (e.g., $1 per 1M tokens)
	tracker := llm.NewCostTracker(1.0, 1.0)
	
	// Set budget limit to $10.00
	guard := llm.NewBudgetGuard(tracker, 10.0)

	// Test 1: Successful Reservation
	err := guard.Reserve(5.0)
	if err != nil {
		t.Fatalf("expected successful reservation, got error: %v", err)
	}

	if guard.Remaining() != 5.0 {
		t.Errorf("expected 5.0 remaining budget, got %f", guard.Remaining())
	}

	// Test 2: Reject Over Budget
	err = guard.Reserve(6.0)
	if err == nil {
		t.Fatal("expected error when reserving more than remaining budget")
	}

	// Test 3: Commit releases reservation and registers actual usage
	guard.Commit(5.0)
	
	// Simulate LLM usage consuming exactly $5.00
	tracker.RecordUsage(2500000, 2500000) 
	
	if guard.Remaining() != 5.0 {
		t.Errorf("expected 5.0 remaining budget after commit/usage, got %f", guard.Remaining())
	}
}