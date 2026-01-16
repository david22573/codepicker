package server

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/david22573/codepicker/internal/logger"
)

type Middleware func(http.HandlerFunc) http.HandlerFunc

func (s *AgentServer) Chain(h http.HandlerFunc, middleware ...Middleware) http.HandlerFunc {
	for i := len(middleware) - 1; i >= 0; i-- {
		h = middleware[i](h)
	}
	return h
}

func RequestID() Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			reqID := fmt.Sprintf("req_%d", time.Now().UnixNano())
			ctx := context.WithValue(r.Context(), "request_id", reqID)
			w.Header().Set("X-Request-ID", reqID)
			next(w, r.WithContext(ctx))
		}
	}
}

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

func EnableCORS(allowedOrigins []string) Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// If no origins configured, default to strictly local or none (dev mode usually needs *)
			// Better safe default: if empty, only allow same-origin (no headers sent)
			allow := false

			if len(allowedOrigins) > 0 {
				if slices.Contains(allowedOrigins, "*") {
					allow = true
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else if slices.Contains(allowedOrigins, origin) {
					allow = true
					w.Header().Set("Access-Control-Allow-Origin", origin)
				}
			}

			if allow {
				w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, DELETE")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			}

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next(w, r)
		}
	}
}

func BodyLimitMiddleware(maxBytes int64) Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next(w, r)
		}
	}
}

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
