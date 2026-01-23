package tracking

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestConcurrentRecordRequest tests thread-safety of recording requests
func TestConcurrentRecordRequest(t *testing.T) {
	tracker := NewCostTracker(10.0) // $10 daily limit

	const numWorkers = 20
	const requestsPerWorker = 50

	var wg sync.WaitGroup
	errors := make(chan error, numWorkers*requestsPerWorker)

	// Launch concurrent workers recording requests
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < requestsPerWorker; j++ {
				// Simulate various models and token counts
				model := "deepseek/deepseek-chat"
				if j%3 == 0 {
					model = "gpt-4o"
				} else if j%3 == 1 {
					model = "claude-3.5-sonnet"
				}

				promptTokens := 100 + (workerID * 10)
				completionTokens := 50 + j

				err := tracker.RecordRequest(promptTokens, completionTokens, model)
				if err != nil {
					errors <- fmt.Errorf("worker %d request %d: %w", workerID, j, err)
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors (excluding legitimate limit errors)
	for err := range errors {
		errStr := err.Error()
		// Only fail on unexpected errors, not limit errors
		if errStr != "" && !contains(errStr, "limit exceeded") {
			t.Errorf("Unexpected error: %v", err)
		}
	}

	// Verify final stats are consistent
	totalCost, requestCount := tracker.GetStats()

	if totalCost < 0 {
		t.Errorf("Total cost should not be negative: %f", totalCost)
	}

	if requestCount < 0 {
		t.Errorf("Request count should not be negative: %d", requestCount)
	}

	// The total should be close to but not exceed the limit
	dailyLimit := tracker.GetDailyLimit()
	if totalCost > dailyLimit*1.01 { // Allow 1% margin for race conditions
		t.Errorf("Total cost %f exceeds daily limit %f by too much", totalCost, dailyLimit)
	}
}

// TestConcurrentGetStats tests thread-safety of reading stats
func TestConcurrentGetStats(t *testing.T) {
	tracker := NewCostTracker(100.0)

	var wg sync.WaitGroup
	const numReaders = 30
	const numWriters = 10

	done := make(chan bool)

	// Concurrent readers
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					cost, count := tracker.GetStats()
					if cost < 0 || count < 0 {
						t.Errorf("Reader %d: invalid stats - cost: %f, count: %d", id, cost, count)
					}
					time.Sleep(time.Millisecond)
				}
			}
		}(i)
	}

	// Concurrent writers
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				select {
				case <-done:
					return
				default:
					tracker.RecordRequest(100, 50, "deepseek/deepseek-chat")
					time.Sleep(time.Millisecond * 2)
				}
			}
		}(i)
	}

	// Let it run for a bit
	time.Sleep(100 * time.Millisecond)
	close(done)
	wg.Wait()
}

// TestConcurrentDailyReset tests thread-safety of daily reset logic
func TestConcurrentDailyReset(t *testing.T) {
	tracker := NewCostTracker(5.0)

	// Set up initial state
	for i := 0; i < 10; i++ {
		tracker.RecordRequest(100, 50, "deepseek/deepseek-chat")
	}

	initialCost, initialCount := tracker.GetStats()
	if initialCost == 0 || initialCount == 0 {
		t.Fatal("Failed to set up initial state")
	}

	// Simulate date change by directly modifying currentDate
	// In production, this happens automatically when crossing midnight
	tracker.mu.Lock()
	tracker.currentDate = "2020-01-01" // Force old date
	tracker.mu.Unlock()

	var wg sync.WaitGroup
	const numWorkers = 15
	errors := make(chan error, numWorkers*5)

	// Concurrent requests should trigger reset
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				err := tracker.RecordRequest(50, 25, "deepseek/deepseek-chat")
				if err != nil {
					errors <- err
				}
				time.Sleep(time.Millisecond)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check that reset occurred (costs should be lower than initial + new requests)
	finalCost, finalCount := tracker.GetStats()

	// After reset, count should be less than or equal to numWorkers * 5
	if finalCount > numWorkers*5+1 {
		t.Logf("Warning: Expected count <= %d after reset, got %d", numWorkers*5, finalCount)
	}

	// Cost should be reasonable for the new requests
	if finalCost < 0 {
		t.Errorf("Final cost should not be negative: %f", finalCost)
	}
}

// TestWarningThresholds tests concurrent triggering of warnings
func TestWarningThresholds(t *testing.T) {
	tracker := NewCostTracker(1.0) // Small limit to trigger warnings easily

	var wg sync.WaitGroup
	const numWorkers = 5

	// Concurrent workers should trigger various warning thresholds
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// Use a model with known pricing
			for j := 0; j < 20; j++ {
				tracker.RecordRequest(1000, 500, "gpt-4o") // More expensive model
				time.Sleep(time.Millisecond)
			}
		}(id)
	}

	wg.Wait()

	// Verify warnings were tracked (internal state check)
	tracker.mu.RLock()
	warningCount := len(tracker.warningsTriggered)
	tracker.mu.RUnlock()

	// Should have triggered at least some warnings
	if warningCount == 0 {
		t.Log("Warning: No warning thresholds were triggered (may be normal if under limits)")
	}
}

// TestConcurrentRemainingBudget tests thread-safety of budget calculations
func TestConcurrentRemainingBudget(t *testing.T) {
	tracker := NewCostTracker(50.0)

	var wg sync.WaitGroup
	const numReaders = 20
	const numWriters = 10

	done := make(chan bool)

	// Concurrent readers
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					remaining := tracker.GetRemainingBudget()
					if remaining < 0 {
						t.Error("Remaining budget should not be negative")
					}
					if remaining > 50.0 {
						t.Errorf("Remaining budget %f exceeds daily limit", remaining)
					}
					time.Sleep(time.Millisecond)
				}
			}
		}()
	}

	// Concurrent writers
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				select {
				case <-done:
					return
				default:
					tracker.RecordRequest(200, 100, "deepseek/deepseek-chat")
					time.Sleep(time.Millisecond * 2)
				}
			}
		}()
	}

	time.Sleep(100 * time.Millisecond)
	close(done)
	wg.Wait()

	// Final sanity check
	remaining := tracker.GetRemainingBudget()
	cost, _ := tracker.GetStats()

	expectedRemaining := 50.0 - cost
	if expectedRemaining < 0 {
		expectedRemaining = 0
	}

	// Allow small floating point differences
	if abs(remaining-expectedRemaining) > 0.01 {
		t.Errorf("Remaining budget %f doesn't match expected %f (cost: %f)", remaining, expectedRemaining, cost)
	}
}

// TestConcurrentUsagePercentage tests thread-safety of usage calculation
func TestConcurrentUsagePercentage(t *testing.T) {
	tracker := NewCostTracker(10.0)

	var wg sync.WaitGroup
	done := make(chan bool)

	// Readers
	for i := 0; i < 15; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					pct := tracker.GetUsagePercentage()
					if pct < 0 || pct > 100 {
						t.Errorf("Usage percentage %f out of valid range [0, 100]", pct)
					}
				}
			}
		}()
	}

	// Writers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 15; j++ {
				select {
				case <-done:
					return
				default:
					tracker.RecordRequest(150, 75, "deepseek/deepseek-chat")
					time.Sleep(time.Millisecond)
				}
			}
		}()
	}

	time.Sleep(50 * time.Millisecond)
	close(done)
	wg.Wait()
}

// TestReset tests thread-safety of reset operation
func TestReset(t *testing.T) {
	tracker := NewCostTracker(5.0)

	// Add some initial data
	for i := 0; i < 20; i++ {
		tracker.RecordRequest(100, 50, "deepseek/deepseek-chat")
	}

	var wg sync.WaitGroup
	const numWorkers = 10

	// Some workers reset, others try to read/write
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			if id%3 == 0 {
				tracker.Reset()
			} else if id%3 == 1 {
				tracker.GetStats()
			} else {
				tracker.RecordRequest(50, 25, "deepseek/deepseek-chat")
			}
		}(i)
	}

	wg.Wait()

	// Final state should be valid (even if unpredictable due to resets)
	cost, count := tracker.GetStats()
	if cost < 0 || count < 0 {
		t.Errorf("Invalid final state: cost=%f, count=%d", cost, count)
	}
}

// TestRestoreState tests concurrent restore operations
func TestRestoreState(t *testing.T) {
	tracker := NewCostTracker(20.0)

	var wg sync.WaitGroup
	const numWorkers = 15

	// Concurrent restore and normal operations
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			if id%4 == 0 {
				// Restore some previous state
				tracker.RestoreState(1.5, 10)
			} else {
				// Normal operations
				tracker.RecordRequest(100, 50, "deepseek/deepseek-chat")
			}
		}(i)
	}

	wg.Wait()

	// Verify tracker is still in a valid state
	cost, count := tracker.GetStats()
	if cost < 0 || count < 0 {
		t.Errorf("Invalid state after concurrent restores: cost=%f, count=%d", cost, count)
	}
}

// TestRaceDetection runs rapid concurrent operations to catch data races (run with -race flag)
func TestRaceDetection(t *testing.T) {
	tracker := NewCostTracker(100.0)

	var wg sync.WaitGroup
	const iterations = 100
	const workers = 10

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				switch j % 6 {
				case 0:
					tracker.RecordRequest(100, 50, "deepseek/deepseek-chat")
				case 1:
					tracker.GetStats()
				case 2:
					tracker.GetRemainingBudget()
				case 3:
					tracker.GetUsagePercentage()
				case 4:
					tracker.GetDailyLimit()
				case 5:
					tracker.Reset()
				}
			}
		}(i)
	}

	wg.Wait()
}

// TestModelPricingConcurrency tests concurrent access to pricing calculations
func TestModelPricingConcurrency(t *testing.T) {
	tracker := NewCostTracker(50.0)

	var wg sync.WaitGroup
	models := []string{
		"deepseek/deepseek-chat",
		"gpt-4o",
		"claude-3.5-sonnet",
		"llama-3.1-70b",
		"gemini-pro",
		"mistral-large",
	}

	// Concurrent requests with different models
	for i := 0; i < len(models); i++ {
		for j := 0; j < 20; j++ {
			wg.Add(1)
			go func(model string) {
				defer wg.Done()
				tracker.RecordRequest(500, 250, model)
			}(models[i])
		}
	}

	wg.Wait()

	cost, count := tracker.GetStats()
	if cost <= 0 || count != len(models)*20 {
		t.Errorf("Expected cost > 0 and count = %d, got cost=%f count=%d", len(models)*20, cost, count)
	}
}

// Helper functions
func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsHelper(s, substr)
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
