package context

import (
	"context"
)

// WatchContext monitors a context and executes a cleanup function if cancelled.
// It returns a stop function that should be called to release resources if the
// operation completes successfully before the context is cancelled.
func WatchContext(ctx context.Context, cleanupFn func()) (stopFn func()) {
	// Create a channel to signal that we should stop watching
	done := make(chan struct{})

	go func() {
		select {
		case <-ctx.Done():
			// Context was cancelled, trigger cleanup
			if ctx.Err() != nil {
				cleanupFn()
			}
		case <-done:
			// Stop function was called, exit goroutine cleanly
			return
		}
	}()

	return func() {
		close(done)
	}
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
