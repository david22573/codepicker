package tracking_test

import (
	"sync"
	"testing"

	"github.com/david22573/codepicker/internal/tracking"
)

func TestCostCalculation(t *testing.T) {
	// Setup tracker with high limit
	tracker := tracking.NewCostTracker(100.0)

	// Test case: GPT-4o pricing check
	// Assumed rates: $2.50 input / $10.00 output per 1M tokens
	// 1000 input tokens = $0.0025
	// 1000 output tokens = $0.0100
	// Total = $0.0125
	err := tracker.RecordRequest(1000, 1000, "gpt-4o")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	cost, count := tracker.GetStats()
	expectedCost := 0.0125

	// Float comparison with epsilon
	if cost < expectedCost-0.000001 || cost > expectedCost+0.000001 {
		t.Errorf("Expected cost $%.6f, got $%.6f", expectedCost, cost)
	}
	if count != 1 {
		t.Errorf("Expected 1 request, got %d", count)
	}
}

func TestDailyLimit(t *testing.T) {
	limit := 0.05
	tracker := tracking.NewCostTracker(limit)

	// First request: OK (Cost ~0.0125)
	if err := tracker.RecordRequest(1000, 1000, "gpt-4o"); err != nil {
		t.Fatalf("First request failed: %v", err)
	}

	// Second request: OK (Total ~0.025)
	if err := tracker.RecordRequest(1000, 1000, "gpt-4o"); err != nil {
		t.Fatalf("Second request failed: %v", err)
	}

	// Third request: Massive usage (Try to add ~$0.25)
	err := tracker.RecordRequest(100000, 0, "gpt-4o")
	if err == nil {
		t.Error("Expected error due to exceeding limit, got nil")
	}

	// Stats should reflect previous valid state and NOT the failed request
	cost, _ := tracker.GetStats()
	if cost > limit {
		t.Errorf("Total cost $%.4f exceeds limit $%.4f despite blocking error", cost, limit)
	}
}

func TestThreadSafety(t *testing.T) {
	tracker := tracking.NewCostTracker(100.0)
	var wg sync.WaitGroup

	// 100 concurrent requests
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tracker.RecordRequest(10, 10, "gpt-3.5-turbo")
		}()
	}

	wg.Wait()

	_, count := tracker.GetStats()
	if count != 100 {
		t.Errorf("Expected 100 requests, got %d", count)
	}
}

func TestReset(t *testing.T) {
	tracker := tracking.NewCostTracker(10.0)
	tracker.RecordRequest(100, 100, "gpt-4o")

	tracker.Reset()

	cost, count := tracker.GetStats()
	if cost != 0 || count != 0 {
		t.Errorf("Reset failed. Cost: %f, Count: %d", cost, count)
	}
}
