package unit

import (
	"testing"

	"[github.com/david22573/codepicker/adapters/agent](https://github.com/david22573/codepicker/adapters/agent)"
	"[github.com/david22573/codepicker/infra/llm](https://github.com/david22573/codepicker/infra/llm)"
)

func TestAdaptiveController_BaseTurnsCalculation(t *testing.T) {
	tracker := llm.NewCostTracker(0.14, 0.28)
	ctrl := agent.NewAdaptiveController(5, 100, tracker, 10.0)
	
	turns := ctrl.CalculateAllowedTurns(0.5)
	
	if turns < 5 {
		t.Errorf("expected >= 5 turns, got %d", turns)
	}
}

func TestAdaptiveController_BudgetDepleted(t *testing.T) {
	tracker := llm.NewCostTracker(0.14, 0.28)
	// Simulate blowing the budget
	tracker.RecordUsage(100000000, 100000000) 
	
	ctrl := agent.NewAdaptiveController(10, 100, tracker, 1.0)
	
	turns := ctrl.CalculateAllowedTurns(0.5)
	
	if turns != 0 {
		t.Errorf("expected 0 turns when budget depleted, got %d", turns)
	}
}