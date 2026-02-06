package llm

import (
	"fmt"
	"sync"
	"time"
)

type State string

const (
	StateClosed   State = "closed"    // Normal operation (requests flow freely)
	StateOpen     State = "open"      // Failure threshold reached (requests blocked)
	StateHalfOpen State = "half-open" // Probation period (testing if service recovered)
)

type CircuitBreaker struct {
	mu               sync.Mutex
	state            State
	failureCount     int
	failureThreshold int
	resetTimeout     time.Duration
	lastFailureTime  time.Time
}

// NewCircuitBreaker initializes a breaker with a specific failure count and cooldown.
func NewCircuitBreaker(threshold int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:            StateClosed,
		failureThreshold: threshold,
		resetTimeout:     timeout,
	}
}

// Execute runs the given function behind the circuit breaker protection.
func (cb *CircuitBreaker) Execute(fn func() error) error {
	cb.mu.Lock()
	if cb.state == StateOpen {
		// Check if enough time has passed to try again
		if time.Since(cb.lastFailureTime) > cb.resetTimeout {
			cb.state = StateHalfOpen
		} else {
			cb.mu.Unlock()
			return fmt.Errorf("circuit breaker is open: LLM service temporarily disabled")
		}
	}
	cb.mu.Unlock()

	// Execute the actual operation (e.g., the LLM API call)
	err := fn()

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.failureCount++
		cb.lastFailureTime = time.Now()

		// If we hit the limit, open the circuit
		if cb.failureCount >= cb.failureThreshold {
			cb.state = StateOpen
		}

		// If we were testing the waters (Half-Open) and failed, go back to Open immediately
		if cb.state == StateHalfOpen {
			cb.state = StateOpen
			// Reset timer to force full wait
			cb.lastFailureTime = time.Now()
		}

		return err
	}

	// Success! Reset everything
	if cb.state == StateHalfOpen {
		// If we succeeded in Half-Open, the service is healthy again
		cb.state = StateClosed
		cb.failureCount = 0
	} else if cb.state == StateClosed {
		// Normal success, just keep failure count clean
		cb.failureCount = 0
	}

	return nil
}
