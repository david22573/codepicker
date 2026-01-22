package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/david22573/codepicker/internal/logger"
)

type Middleware func(http.HandlerFunc) http.HandlerFunc

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

func AuthMiddleware(token string) Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Unauthorized: Missing Authorization header", http.StatusUnauthorized)
				return
			}

			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				http.Error(w, "Unauthorized: Invalid header format", http.StatusUnauthorized)
				return
			}

			if parts[1] != token {
				http.Error(w, "Unauthorized: Invalid token", http.StatusUnauthorized)
				return
			}

			next(w, r)
		}
	}
}

func EnableCORS(allowedOrigins []string) Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
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
				w.Header().Set("Access-Control-Allow-Methods", "POST, GET, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
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
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
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
					if os.Getenv("CODEPICKER_DEBUG") == "true" {
						fmt.Printf("Trace: %v\n", err)
					}
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			}()
			next(w, r)
		}
	}
}
