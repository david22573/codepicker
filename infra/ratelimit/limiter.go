package ratelimit

import (
	"context"
	"sync"

	"golang.org/x/time/rate"
)

// ToolRateLimiter manages execution frequency per tool type.
type ToolRateLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	defaultR rate.Limit
	burst    int
}

// NewToolRateLimiter initializes a limiter with a default operations-per-second.
func NewToolRateLimiter(opsPerSecond int) *ToolRateLimiter {
	return &ToolRateLimiter{
		limiters: make(map[string]*rate.Limiter),
		defaultR: rate.Limit(opsPerSecond),
		burst:    1,
	}
}

// Wait blocks until the tool is allowed to execute or the context is cancelled.
func (l *ToolRateLimiter) Wait(ctx context.Context, toolName string) error {
	l.mu.Lock()
	limiter, exists := l.limiters[toolName]
	if !exists {
		limiter = rate.NewLimiter(l.defaultR, l.burst)
		l.limiters[toolName] = limiter
	}
	l.mu.Unlock()

	return limiter.Wait(ctx)
}

// SetLimit allows dynamic configuration of limits per tool.
func (l *ToolRateLimiter) SetLimit(toolName string, opsPerSecond int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.limiters[toolName] = rate.NewLimiter(rate.Limit(opsPerSecond), l.burst)
}
