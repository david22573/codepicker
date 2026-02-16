package agent

import (
	"math"

	"github.com/david22573/codepicker/infra/llm"
)

// AdaptiveController manages the agent's turn budget based on real-time feedback.
type AdaptiveController struct {
	baseTurns   int
	maxTurns    int
	costTracker *llm.CostTracker
	budgetLimit float64
}

func NewAdaptiveController(base, max int, tracker *llm.CostTracker, limit float64) *AdaptiveController {
	return &AdaptiveController{
		baseTurns:   base,
		maxTurns:    max,
		costTracker: tracker,
		budgetLimit: limit,
	}
}

// CalculateAllowedTurns returns how many turns the agent can take.
func (c *AdaptiveController) CalculateAllowedTurns(taskComplexity float64) int {
	currentCost := c.costTracker.GetMetrics().TotalCost

	// FIX: Hard stop if over budget.
	// Previously, the ratio calc below would allow 10% turns even when over budget.
	if c.budgetLimit > 0 && currentCost >= c.budgetLimit {
		return 0
	}

	// If we are close to budget, restrict turns
	budgetFactor := 1.0
	if currentCost > 0 {
		remainingRatio := (c.budgetLimit - currentCost) / c.budgetLimit
		budgetFactor = math.Max(0.1, remainingRatio)
	}

	// Complexity scales turns from base up to max
	calculated := float64(c.baseTurns) + (taskComplexity * float64(c.maxTurns-c.baseTurns))

	finalTurns := int(calculated * budgetFactor)
	if finalTurns > c.maxTurns {
		return c.maxTurns
	}
	if finalTurns < 1 {
		return 1
	}
	return finalTurns
}
