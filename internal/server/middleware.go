package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/david22573/codepicker/internal/logger"
)

type Middleware func(http.HandlerFunc) http.HandlerFunc

// Chain applies middlewares in reverse order (outermost first)
func (s *AgentServer) Chain(h http.HandlerFunc, middleware ...Middleware) http.HandlerFunc {
	for i := len(middleware) - 1; i >= 0; i-- {
		h = middleware[i](h)
	}
	return h
}

// RequestID adds a unique ID to the context and response headers
func RequestID() Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// Simple timestamp-based ID for now
			reqID := fmt.Sprintf("req_%d", time.Now().UnixNano())

			// Add to context
			ctx := context.WithValue(r.Context(), "request_id", reqID)

			// Add to response header for client debugging
			w.Header().Set("X-Request-ID", reqID)

			next(w, r.WithContext(ctx))
		}
	}
}

// RequestLogger logs incoming requests with duration and ID
func RequestLogger(log logger.Logger) Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			reqID := r.Context().Value("request_id")

			log.Info(fmt.Sprintf("[%s] -> %s %s", reqID, r.Method, r.URL.Path))

			next(w, r)

			duration := time.Since(start)
			log.Debug(fmt.Sprintf("[%s] <- Completed in %v", reqID, duration))
		}
	}
}

// EnableCORS handles Cross-Origin Resource Sharing
func EnableCORS() Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, DELETE")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next(w, r)
		}
	}
}

// RecoveryMiddleware catches panics and prevents server crash
func RecoveryMiddleware(log logger.Logger) Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					log.Error(fmt.Sprintf("PANIC RECOVERED: %v", err))
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			}()
			next(w, r)
		}
	}
}
