package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// TaskDuration tracks how long agent tasks take to complete.
	TaskDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "codepicker_task_duration_seconds",
		Help: "Duration of agent tasks in seconds",
	}, []string{"status"})

	// ToolCallRate tracks frequency of tool usage.
	ToolCallRate = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "codepicker_tool_calls_total",
		Help: "Total number of tool calls",
	}, []string{"tool"})

	// LLMCost tracks the accumulated USD spend.
	LLMCost = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "codepicker_llm_cost_usd",
		Help: "Total estimated LLM cost in USD",
	})

	// TokenUsage tracks prompt vs completion tokens.
	TokenUsage = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "codepicker_llm_tokens_total",
		Help: "Total tokens consumed by LLM",
	}, []string{"type"})

	// PolicyViolations tracks how often the guardrails trigger.
	PolicyViolations = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "codepicker_policy_violations_total",
		Help: "Total number of policy violations",
	}, []string{"reason"})
)
