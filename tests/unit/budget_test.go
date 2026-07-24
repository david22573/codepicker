package unit

import (
	"testing"

	"github.com/david22573/codepicker/infra/llm"
	"github.com/david22573/codepicker/runtime"
)

func init() {
	runtime.Global.Mode = runtime.ModeProduction
}

func TestBudgetGuard_ReserveAndCommit(t *testing.T) {
	tracker := llm.NewCostTracker(0.14, 0.28)
	guard := llm.NewBudgetGuard(tracker, 1.0) // $1.00 limit

	// Test 1: Successful reservation
	err := guard.Reserve(0.50)
	if err != nil {
		t.Fatalf("expected successful reservation, got error: %v", err)
	}

	remaining := guard.Remaining()
	if remaining != 0.50 {
		t.Errorf("expected $0.50 remaining, got $%.2f", remaining)
	}

	// Test 2: Reject over-budget reservation
	err = guard.Reserve(0.60)
	if err == nil {
		t.Fatal("expected error when reserving over budget, got nil")
	}

	// Test 3: Commit releases reservation
	guard.Commit(0.50)
	remaining = guard.Remaining()
	if remaining != 1.00 {
		t.Errorf("expected full $1.00 remaining after commit, got $%.2f", remaining)
	}
}

func TestBudgetGuard_ZeroLimitIsInfinite(t *testing.T) {
	tracker := llm.NewCostTracker(0.14, 0.28)
	guard := llm.NewBudgetGuard(tracker, 0) // $0 limit = infinite

	err := guard.Reserve(9999.0)
	if err != nil {
		t.Fatalf("expected successful reservation on infinite budget, got: %v", err)
	}
}

func TestBudgetGuard_TracksActualUsage(t *testing.T) {
	tracker := llm.NewCostTracker(1.0, 1.0)   // $1 per 1M tokens
	guard := llm.NewBudgetGuard(tracker, 2.0) // $2.00 limit

	// Record $1.00 worth of usage
	tracker.RecordUsage(500_000, 500_000)

	// Attempt to reserve $1.50 (Total: $2.50, exceeds $2.00 limit)
	err := guard.Reserve(1.50)
	if err == nil {
		t.Fatal("expected error when combined usage and reservation exceeds limit")
	}

	// Attempt to reserve $0.50 (Total: $1.50, allowed)
	err = guard.Reserve(0.50)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
}
