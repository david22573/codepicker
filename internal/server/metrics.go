package server

import (
	"fmt"
	"net/http"
	"runtime"
)

func (s *AgentServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	cost, count := s.Engine.CostTracker.GetStats()

	// Prometheus format
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	fmt.Fprintf(w, "# HELP codepicker_requests_total Total number of agent requests processed\n")
	fmt.Fprintf(w, "# TYPE codepicker_requests_total counter\n")
	fmt.Fprintf(w, "codepicker_requests_total %d\n", count)

	fmt.Fprintf(w, "# HELP codepicker_cost_usd_total Total accumulated cost in USD\n")
	fmt.Fprintf(w, "# TYPE codepicker_cost_usd_total counter\n")
	fmt.Fprintf(w, "codepicker_cost_usd_total %f\n", cost)

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Fprintf(w, "# HELP go_goroutines Number of goroutines that currently exist\n")
	fmt.Fprintf(w, "# TYPE go_goroutines gauge\n")
	fmt.Fprintf(w, "go_goroutines %d\n", runtime.NumGoroutine())

	fmt.Fprintf(w, "# HELP go_mem_alloc_bytes Number of bytes allocated and still in use\n")
	fmt.Fprintf(w, "# TYPE go_mem_alloc_bytes gauge\n")
	fmt.Fprintf(w, "go_mem_alloc_bytes %d\n", m.Alloc)
}
