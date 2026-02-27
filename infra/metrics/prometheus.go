package metrics

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// PrometheusBackend implements the Metrics interface dynamically.
type PrometheusBackend struct {
	counters   map[string]*prometheus.CounterVec
	histograms map[string]*prometheus.HistogramVec
	gauges     map[string]*prometheus.GaugeVec
	mu         sync.RWMutex
}

func NewPrometheusBackend() *PrometheusBackend {
	return &PrometheusBackend{
		counters:   make(map[string]*prometheus.CounterVec),
		histograms: make(map[string]*prometheus.HistogramVec),
		gauges:     make(map[string]*prometheus.GaugeVec),
	}
}

func (p *PrometheusBackend) getLabelNames(labels map[string]string) []string {
	var names []string
	for k := range labels {
		names = append(names, k)
	}
	return names
}

func (p *PrometheusBackend) IncCounter(name string, labels map[string]string) {
	p.AddCounter(name, 1, labels)
}

func (p *PrometheusBackend) AddCounter(name string, value float64, labels map[string]string) {
	p.mu.RLock()
	vec, exists := p.counters[name]
	p.mu.RUnlock()

	if !exists {
		p.mu.Lock()
		vec, exists = p.counters[name]
		if !exists {
			vec = promauto.NewCounterVec(prometheus.CounterOpts{
				Name: name,
				Help: "Auto-generated counter for " + name,
			}, p.getLabelNames(labels))
			p.counters[name] = vec
		}
		p.mu.Unlock()
	}

	if len(labels) == 0 {
		vec.WithLabelValues().Add(value)
	} else {
		vec.With(labels).Add(value)
	}
}

func (p *PrometheusBackend) ObserveDuration(name string, duration time.Duration) {
	p.mu.RLock()
	vec, exists := p.histograms[name]
	p.mu.RUnlock()

	if !exists {
		p.mu.Lock()
		vec, exists = p.histograms[name]
		if !exists {
			vec = promauto.NewHistogramVec(prometheus.HistogramOpts{
				Name: name,
				Help: "Auto-generated histogram for " + name,
				// Granular buckets for both fast tool calls and slow LLM responses
				Buckets: []float64{0.1, 0.5, 1.0, 2.0, 5.0, 10.0, 30.0, 60.0, 120.0},
			}, []string{})
			p.histograms[name] = vec
		}
		p.mu.Unlock()
	}

	vec.WithLabelValues().Observe(duration.Seconds())
}

func (p *PrometheusBackend) ObserveValue(name string, value float64) {
	p.mu.RLock()
	vec, exists := p.gauges[name]
	p.mu.RUnlock()

	if !exists {
		p.mu.Lock()
		vec, exists = p.gauges[name]
		if !exists {
			vec = promauto.NewGaugeVec(prometheus.GaugeOpts{
				Name: name,
				Help: "Auto-generated gauge for " + name,
			}, []string{})
			p.gauges[name] = vec
		}
		p.mu.Unlock()
	}

	vec.WithLabelValues().Set(value)
}

// Keep existing static metrics for backward compatibility with older dashboards
var (
	TaskDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "codepicker_task_duration_seconds",
		Help: "Duration of agent tasks in seconds",
	}, []string{"status"})

	ToolCallRate = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "codepicker_tool_calls_total",
		Help: "Total number of tool calls",
	}, []string{"tool"})

	LLMCost = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "codepicker_llm_cost_usd",
		Help: "Total estimated LLM cost in USD",
	})

	TokenUsage = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "codepicker_llm_tokens_total",
		Help: "Total tokens consumed by LLM",
	}, []string{"type"})

	PolicyViolations = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "codepicker_policy_violations_total",
		Help: "Total number of policy violations",
	}, []string{"reason"})
)