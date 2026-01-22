package server

import (
	"net/http"
	"sync"

	"golang.org/x/time/rate"
)

// RateLimiter manages rate limiting for the server.
// Currently implements a global limiter, but can be extended to per-IP.
type RateLimiter struct {
	globalLimiter *rate.Limiter
	mu            sync.Mutex
}

func NewRateLimiter(r rate.Limit, b int) *RateLimiter {
	return &RateLimiter{
		globalLimiter: rate.NewLimiter(r, b),
	}
}

func (l *RateLimiter) Middleware() Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !l.globalLimiter.Allow() {
				// Retry-After logic
				w.Header().Set("Retry-After", "60")
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}
			next(w, r)
		}
	}
}
