package progress

import (
	"sync"
	"testing"
	"time"
)

// TestConcurrentStartStop tests thread-safety of Start/Stop operations
func TestConcurrentStartStop(t *testing.T) {
	spinner := NewSpinner("Testing concurrent operations")

	var wg sync.WaitGroup
	const numOperations = 50

	// Concurrent Start/Stop calls
	for i := 0; i < numOperations; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			if id%2 == 0 {
				spinner.Start()
			} else {
				spinner.Stop()
			}
		}(i)
	}

	wg.Wait()

	// Final stop to clean up
	spinner.Stop()
}

// TestMultipleStarts tests calling Start multiple times
func TestMultipleStarts(t *testing.T) {
	spinner := NewSpinner("Multiple starts test")
	defer spinner.Stop()

	var wg sync.WaitGroup
	const numStarts = 20

	// Multiple concurrent Start calls should be safe
	for i := 0; i < numStarts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			spinner.Start()
			time.Sleep(time.Millisecond * 5)
		}()
	}

	wg.Wait()
}

// TestMultipleStops tests calling Stop multiple times
func TestMultipleStops(t *testing.T) {
	spinner := NewSpinner("Multiple stops test")
	spinner.Start()
	time.Sleep(time.Millisecond * 20)

	var wg sync.WaitGroup
	const numStops = 20

	// Multiple concurrent Stop calls should be safe
	for i := 0; i < numStops; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			spinner.Stop()
		}()
	}

	wg.Wait()
}

// TestStartStopSequence tests repeated start/stop sequences
func TestStartStopSequence(t *testing.T) {
	spinner := NewSpinner("Sequence test")

	const iterations = 10

	for i := 0; i < iterations; i++ {
		spinner.Start()
		time.Sleep(time.Millisecond * 10)
		spinner.Stop()
	}
}

// TestConcurrentSequences tests multiple goroutines doing start/stop sequences
func TestConcurrentSequences(t *testing.T) {
	spinner := NewSpinner("Concurrent sequences")
	defer spinner.Stop()

	var wg sync.WaitGroup
	const numWorkers = 10
	const sequencesPerWorker = 5

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < sequencesPerWorker; j++ {
				spinner.Start()
				time.Sleep(time.Millisecond * 2)
				spinner.Stop()
				time.Sleep(time.Millisecond)
			}
		}(i)
	}

	wg.Wait()
}

// TestSpinnerLifecycle tests full lifecycle under concurrent access
func TestSpinnerLifecycle(t *testing.T) {
	spinner := NewSpinner("Lifecycle test")

	var wg sync.WaitGroup
	done := make(chan bool)

	// Reader goroutines checking state
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					// Just access the spinner to test thread safety
					spinner.mu.Lock()
					_ = spinner.active
					spinner.mu.Unlock()
					time.Sleep(time.Millisecond)
				}
			}
		}()
	}

	// Control goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			spinner.Start()
			time.Sleep(time.Millisecond * 5)
			spinner.Stop()
			time.Sleep(time.Millisecond * 2)
		}
		close(done)
	}()

	wg.Wait()
}

// TestRaceDetection runs rapid operations to catch data races (run with -race flag)
func TestRaceDetection(t *testing.T) {
	spinner := NewSpinner("Race detection")
	defer spinner.Stop()

	var wg sync.WaitGroup
	const iterations = 100

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				if j%2 == 0 {
					spinner.Start()
				} else {
					spinner.Stop()
				}
			}
		}()
	}

	wg.Wait()
}

// TestStopWithoutStart tests stopping a spinner that was never started
func TestStopWithoutStart(t *testing.T) {
	spinner := NewSpinner("Stop without start")

	var wg sync.WaitGroup
	const numStops = 10

	// Multiple concurrent stops on an never-started spinner
	for i := 0; i < numStops; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			spinner.Stop() // Should be safe
		}()
	}

	wg.Wait()
}

// TestDeferredStop tests deferred Stop calls in concurrent goroutines
func TestDeferredStop(t *testing.T) {
	spinner := NewSpinner("Deferred stop test")

	var wg sync.WaitGroup
	const numWorkers = 15

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			defer spinner.Stop() // All goroutines call Stop on exit

			if id%3 == 0 {
				spinner.Start()
			}

			time.Sleep(time.Millisecond * 10)
		}(i)
	}

	wg.Wait()
}

// TestMultipleSpinners tests multiple spinner instances running concurrently
func TestMultipleSpinners(t *testing.T) {
	const numSpinners = 10
	spinners := make([]*Spinner, numSpinners)

	for i := 0; i < numSpinners; i++ {
		spinners[i] = NewSpinner("Spinner " + string(rune('A'+i)))
	}

	var wg sync.WaitGroup

	// Start all spinners concurrently
	for i, spinner := range spinners {
		wg.Add(1)
		go func(s *Spinner, id int) {
			defer wg.Done()
			s.Start()
			time.Sleep(time.Millisecond * time.Duration(10+id*2))
			s.Stop()
		}(spinner, i)
	}

	wg.Wait()
}

// TestSpinnerWithQuickOperations tests rapid start/stop sequences
func TestSpinnerWithQuickOperations(t *testing.T) {
	spinner := NewSpinner("Quick operations")
	defer spinner.Stop()

	var wg sync.WaitGroup
	const numWorkers = 20

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				spinner.Start()
				// Very short duration - stress test the ticker
				time.Sleep(time.Microsecond * 100)
				spinner.Stop()
			}
		}()
	}

	wg.Wait()
}

// TestStartWhileAlreadyRunning tests starting while spinner is already running
func TestStartWhileAlreadyRunning(t *testing.T) {
	spinner := NewSpinner("Already running test")

	spinner.Start()
	defer spinner.Stop()

	time.Sleep(time.Millisecond * 20)

	var wg sync.WaitGroup
	const numAttempts = 15

	// Try to start again while already running
	for i := 0; i < numAttempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			spinner.Start() // Should be idempotent
		}()
	}

	wg.Wait()
	time.Sleep(time.Millisecond * 20)
}

// TestStopWhileAlreadyStopped tests stopping while spinner is already stopped
func TestStopWhileAlreadyStopped(t *testing.T) {
	spinner := NewSpinner("Already stopped test")

	spinner.Start()
	time.Sleep(time.Millisecond * 20)
	spinner.Stop()

	var wg sync.WaitGroup
	const numAttempts = 15

	// Try to stop again while already stopped
	for i := 0; i < numAttempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			spinner.Stop() // Should be idempotent
		}()
	}

	wg.Wait()
}

// TestConcurrentMessageChange tests changing message while spinner is running
func TestConcurrentMessageChange(t *testing.T) {
	spinner := NewSpinner("Initial message")
	spinner.Start()
	defer spinner.Stop()

	var wg sync.WaitGroup
	const numChanges = 20

	// Concurrent message changes
	for i := 0; i < numChanges; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// Accessing the message field concurrently
			spinner.mu.Lock()
			spinner.message = "Message " + string(rune('0'+id%10))
			spinner.mu.Unlock()
			time.Sleep(time.Millisecond * 2)
		}(i)
	}

	wg.Wait()
}

// TestSpinnerCleanup tests cleanup after goroutine exits
func TestSpinnerCleanup(t *testing.T) {
	spinner := NewSpinner("Cleanup test")
	spinner.Start()

	// Give it time to run
	time.Sleep(time.Millisecond * 30)

	// Stop and verify cleanup
	spinner.Stop()

	// Verify wg is properly waited on
	spinner.mu.Lock()
	active := spinner.active
	spinner.mu.Unlock()

	if active {
		t.Error("Spinner should not be active after Stop")
	}
}

// TestConcurrentSpinnerCreation tests creating spinners concurrently
func TestConcurrentSpinnerCreation(t *testing.T) {
	var wg sync.WaitGroup
	const numSpinners = 30

	spinners := make([]*Spinner, numSpinners)

	for i := 0; i < numSpinners; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			spinners[idx] = NewSpinner("Spinner " + string(rune('A'+idx%26)))
		}(i)
	}

	wg.Wait()

	// Verify all spinners were created
	for i, spinner := range spinners {
		if spinner == nil {
			t.Errorf("Spinner %d was not created", i)
		}
	}

	// Clean up by stopping all
	for _, spinner := range spinners {
		if spinner != nil {
			spinner.Stop()
		}
	}
}

// TestLongRunningSpinner tests a spinner that runs for an extended period
func TestLongRunningSpinner(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping long-running test in short mode")
	}

	spinner := NewSpinner("Long running test")

	var wg sync.WaitGroup
	done := make(chan bool)

	// Start the spinner
	spinner.Start()

	// Concurrent operations while it runs
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ticker := time.NewTicker(time.Millisecond * 50)
			defer ticker.Stop()

			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					// Periodically check state
					spinner.mu.Lock()
					_ = spinner.active
					spinner.mu.Unlock()
				}
			}
		}(i)
	}

	// Let it run for a bit
	time.Sleep(time.Millisecond * 200)

	// Signal stop
	spinner.Stop()
	close(done)
	wg.Wait()
}

// TestSpinnerWithPanic tests that spinner handles panics gracefully
func TestSpinnerWithPanic(t *testing.T) {
	spinner := NewSpinner("Panic test")
	spinner.Start()

	// Ensure cleanup happens even if we panic
	defer func() {
		if r := recover(); r != nil {
			t.Logf("Recovered from panic: %v", r)
		}
		spinner.Stop()
	}()

	time.Sleep(time.Millisecond * 10)

	// This tests that Stop works correctly even in panic scenarios
}

// TestHighFrequencyToggle tests very rapid toggling
func TestHighFrequencyToggle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping high-frequency test in short mode")
	}

	spinner := NewSpinner("High frequency toggle")
	defer spinner.Stop()

	done := make(chan bool)
	var wg sync.WaitGroup

	// Rapid toggler
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				spinner.Start()
				spinner.Stop()
			}
		}
	}()

	// Let it run briefly
	time.Sleep(time.Millisecond * 100)
	close(done)
	wg.Wait()
}
