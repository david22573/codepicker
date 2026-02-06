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
	// 1. Check State (Hold Lock briefly)
	cb.mu.Lock()
	currentState := cb.state
	if currentState == StateOpen {
		if time.Since(cb.lastFailureTime) > cb.resetTimeout {
			cb.state = StateHalfOpen
			currentState = StateHalfOpen
		} else {
			cb.mu.Unlock()
			return fmt.Errorf("circuit breaker is open: LLM service temporarily disabled")
		}
	}
	cb.mu.Unlock()

	// 2. Execute Operation (NO LOCK - Concurrency allowed!)
	// This was the critical fix: previously the lock was held here.
	err := fn()

	// 3. Update State based on result (Hold Lock briefly)
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
		if currentState == StateHalfOpen {
			cb.state = StateOpen
			cb.lastFailureTime = time.Now()
		}

		return err
	}

	// Success! Reset everything if we were shaky or just keep clean
	if currentState == StateHalfOpen || currentState == StateClosed {
		cb.state = StateClosed
		cb.failureCount = 0
	}

	return nil
}
