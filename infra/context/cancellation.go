package context

import (
	"context"
)

// WatchContext monitors a context and executes a cleanup function if cancelled.
// This is used to trigger rollbacks or logging when a user interrupts the agent.
func WatchContext(ctx context.Context, cleanupFn func()) {
	go func() {
		<-ctx.Done()
		if ctx.Err() != nil {
			cleanupFn()
		}
	}()
}

// IsCancelled is a helper to check if the context has been terminated.
func IsCancelled(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}
