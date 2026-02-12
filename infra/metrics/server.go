package metrics

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server manages the observability endpoints (Health, Liveness, Metrics).
type Server struct {
	server *http.Server
	port   int
	mu     sync.Mutex
	// IsReady can be toggled by the application to signal readiness to K8s
	IsReady bool
}

// NewServer creates a new metrics server on the specified port.
func NewServer(port int) *Server {
	s := &Server{
		port:    port,
		IsReady: true, // Default to true, but can be managed externally
	}

	mux := http.NewServeMux()

	// 1. Prometheus Metrics Endpoint
	// Scraped by Prometheus/Grafana to visualize tool usage and costs.
	mux.Handle("/metrics", promhttp.Handler())

	// 2. Liveness Probe (/health)
	// Used by K8s to restart the pod if the process is deadlocked or hung.
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// 3. Readiness Probe (/ready)
	// Used by K8s LoadBalancers. Fails if the app is starting up or overloaded.
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()

		if !s.IsReady {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("NOT_READY"))
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("READY"))
	})

	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  15 * time.Second,
	}

	return s
}

// Start runs the HTTP server in a background goroutine.
// It is non-blocking.
func (s *Server) Start() {
	go func() {
		fmt.Printf("📊 [METRICS] Server listening on :%d\n", s.port)
		// ErrServerClosed is normal during graceful shutdown, so we ignore it.
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("❌ [METRICS] Server failed: %v\n", err)
		}
	}()
}

// Shutdown gracefully stops the metrics server with the provided context timeout.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Mark as not ready immediately
	s.IsReady = false

	fmt.Println("📉 [METRICS] Shutting down...")
	return s.server.Shutdown(ctx)
}

// SetReady allows the application to signal if it can accept traffic.
func (s *Server) SetReady(ready bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.IsReady = ready
}
