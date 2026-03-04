package metrics

import (
	"time"
)

// Metrics defines the pluggable interface for structured runtime observability.
type Metrics interface {
	IncCounter(name string, labels map[string]string)
	AddCounter(name string, value float64, labels map[string]string)
	ObserveDuration(name string, duration time.Duration)
	ObserveValue(name string, value float64)
}

// NoOpMetrics provides a zero-cost default implementation.
type NoOpMetrics struct{}

func (m *NoOpMetrics) IncCounter(name string, labels map[string]string)                {}
func (m *NoOpMetrics) AddCounter(name string, value float64, labels map[string]string) {}
func (m *NoOpMetrics) ObserveDuration(name string, duration time.Duration)             {}
func (m *NoOpMetrics) ObserveValue(name string, value float64)                         {}

// globalRegistry allows simple access across the codebase without polluting constructors.
var globalRegistry Metrics = &NoOpMetrics{}

// SetRegistry configures the global metrics backend (e.g., Prometheus).
func SetRegistry(m Metrics) {
	globalRegistry = m
}

// GetRegistry returns the current metrics backend.
func GetRegistry() Metrics {
	return globalRegistry
}
