package tracking

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

type CostTracker struct {
	mu           sync.RWMutex
	totalCost    float64
	requestCount int
	dailyLimit   float64
	currentDate  string

	// Track warning thresholds that have been triggered
	warningsTriggered map[float64]bool
}

func NewCostTracker(dailyLimit float64) *CostTracker {
	return &CostTracker{
		dailyLimit:        dailyLimit,
		currentDate:       time.Now().Format("2006-01-02"),
		warningsTriggered: make(map[float64]bool),
	}
}

func (ct *CostTracker) RecordRequest(promptTokens, completionTokens int, model string) error {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	// Reset if it's a new day
	today := time.Now().Format("2006-01-02")
	if today != ct.currentDate {
		ct.totalCost = 0
		ct.requestCount = 0
		ct.currentDate = today
		ct.warningsTriggered = make(map[float64]bool) // Reset warnings for new day
	}

	// Calculate cost using improved estimation
	cost := calculateCost(promptTokens, completionTokens, model)

	if ct.totalCost+cost > ct.dailyLimit {
		return fmt.Errorf("daily cost limit exceeded: $%.4f / $%.2f",
			ct.totalCost+cost, ct.dailyLimit)
	}

	// Check warning thresholds
	oldTotal := ct.totalCost
	newTotal := ct.totalCost + cost
	ct.checkAndLogWarnings(oldTotal, newTotal)

	ct.totalCost = newTotal
	ct.requestCount++
	return nil
}

func (ct *CostTracker) checkAndLogWarnings(oldTotal, newTotal float64) {
	// Define warning thresholds as percentages of daily limit
	thresholds := []struct {
		percent float64
		message string
	}{
		{0.5, "50% of daily cost limit reached"},
		{0.75, "75% of daily cost limit reached"},
		{0.9, "90% of daily cost limit reached - approaching limit"},
		{0.95, "95% of daily cost limit reached - limit imminent"},
	}

	for _, t := range thresholds {
		thresholdAmount := ct.dailyLimit * t.percent
		if oldTotal < thresholdAmount && newTotal >= thresholdAmount {
			if !ct.warningsTriggered[t.percent] {
				log.Printf("💰 COST WARNING: %s ($%.4f / $%.2f)", t.message, newTotal, ct.dailyLimit)
				ct.warningsTriggered[t.percent] = true
			}
		}
	}
}

func (ct *CostTracker) GetStats() (float64, int) {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.totalCost, ct.requestCount
}

func (ct *CostTracker) GetDailyLimit() float64 {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.dailyLimit
}

func (ct *CostTracker) GetRemainingBudget() float64 {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	remaining := ct.dailyLimit - ct.totalCost
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (ct *CostTracker) GetUsagePercentage() float64 {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	if ct.dailyLimit == 0 {
		return 0
	}
	return (ct.totalCost / ct.dailyLimit) * 100
}

// calculateCost returns cost in USD based on token counts and model
func calculateCost(promptTokens, completionTokens int, model string) float64 {
	modelLower := strings.ToLower(model)

	// Get pricing rates per 1M tokens
	promptRate, completionRate := getModelPricing(modelLower)

	// Convert from per-million to per-token
	promptCostPerToken := promptRate / 1_000_000
	completionCostPerToken := completionRate / 1_000_000

	inputCost := float64(promptTokens) * promptCostPerToken
	outputCost := float64(completionTokens) * completionCostPerToken

	// Log if using estimated pricing for significant costs
	totalTokens := promptTokens + completionTokens
	if totalTokens > 1000 {
		estimated := isEstimatedPricing(modelLower)
		if estimated {
			log.Printf("💰 Cost estimation: Using estimated pricing for model %q", model)
		}
	}

	return inputCost + outputCost
}

// getModelPricing returns (prompt rate per 1M tokens, completion rate per 1M tokens)
func getModelPricing(model string) (float64, float64) {
	// Default safe upper bound estimate (expensive models)
	defaultPromptRate := 5.0
	defaultCompletionRate := 15.0

	// Check for exact model matches first
	switch {
	case strings.Contains(model, "deepseek/deepseek-chat"):
		return 0.14, 0.28 // $0.14/$0.28 per 1M tokens
	case strings.Contains(model, "deepseek/deepseek-coder"):
		return 0.14, 0.28
	case strings.Contains(model, "deepseek-r1"):
		return 0.14, 0.28
	case strings.Contains(model, "deepseek-v3"):
		return 0.14, 0.28

	case strings.Contains(model, "gpt-4o"):
		return 2.50, 10.00 // $2.5/$10 per 1M tokens
	case strings.Contains(model, "gpt-4-turbo"):
		return 10.00, 30.00
	case strings.Contains(model, "gpt-4"):
		return 30.00, 60.00

	case strings.Contains(model, "gpt-3.5-turbo"):
		return 1.50, 2.00
	case strings.Contains(model, "gpt-3.5"):
		return 1.50, 2.00

	case strings.Contains(model, "claude-3.5-sonnet"):
		return 3.00, 15.00
	case strings.Contains(model, "claude-3-opus"):
		return 15.00, 75.00
	case strings.Contains(model, "claude-3-haiku"):
		return 0.25, 1.25
	case strings.Contains(model, "claude-3"):
		return 3.00, 15.00 // Default to sonnet pricing

	case strings.Contains(model, "llama-3.1-70b"):
		return 0.88, 0.88
	case strings.Contains(model, "llama-3.1-8b"):
		return 0.11, 0.11
	case strings.Contains(model, "llama-3"):
		return 0.59, 0.79
	case strings.Contains(model, "llama-2"):
		return 0.20, 0.20

	case strings.Contains(model, "gemini-pro"):
		return 0.375, 1.125
	case strings.Contains(model, "gemini-1.5"):
		return 1.25, 3.75

	case strings.Contains(model, "mistral-small"):
		return 0.20, 0.20
	case strings.Contains(model, "mistral-medium"):
		return 0.27, 0.27
	case strings.Contains(model, "mistral-large"):
		return 2.00, 6.00
	case strings.Contains(model, "mixtral-8x7b"):
		return 0.24, 0.24
	case strings.Contains(model, "mixtral"):
		return 0.24, 0.24

	case strings.Contains(model, "codellama"):
		return 0.20, 0.20

	case strings.Contains(model, "command-r"):
		return 0.50, 1.50
	case strings.Contains(model, "command-r-plus"):
		return 3.00, 15.00

	default:
		// For unknown models, use conservative defaults
		// Check if it looks like an expensive model
		if strings.Contains(model, "4") || strings.Contains(model, "opus") || strings.Contains(model, "large") {
			return 15.00, 30.00 // Conservative high estimate
		}
		return defaultPromptRate, defaultCompletionRate
	}
}

// isEstimatedPricing returns true if we're using estimated pricing vs exact known pricing
func isEstimatedPricing(model string) bool {
	// These are models we have exact pricing for
	exactModels := []string{
		"deepseek/deepseek-chat",
		"deepseek/deepseek-coder",
		"gpt-4o",
		"gpt-4-turbo",
		"gpt-4",
		"gpt-3.5-turbo",
		"claude-3.5-sonnet",
		"claude-3-opus",
		"claude-3-haiku",
		"llama-3.1-70b",
		"llama-3.1-8b",
		"gemini-pro",
		"gemini-1.5",
		"mistral-small",
		"mistral-medium",
		"mixtral-8x7b",
	}

	for _, exact := range exactModels {
		if strings.Contains(model, exact) {
			return false
		}
	}
	return true
}
