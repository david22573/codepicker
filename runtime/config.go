package runtime

// Config holds centralized magic numbers, budget limits, and tuning parameters.
type Config struct {
	ExpectedOutputTokens int
	DefaultMemoryTokens  int
	MaxObservationLength int
	ScoutBudgetPercent   float64
	AuditBudgetPercent   float64
	ExplainerEstCost     float64
	PatchGenEstCost      float64
	PatchRefineEstCost   float64
	RerankerEstCost      float64
}

// Global holds the active runtime configuration defaults.
var Global = Config{
	ExpectedOutputTokens: 1024,
	DefaultMemoryTokens:  16000,
	MaxObservationLength: 4000,
	ScoutBudgetPercent:   0.20,
	AuditBudgetPercent:   0.80,
	ExplainerEstCost:     0.002,
	PatchGenEstCost:      0.005,
	PatchRefineEstCost:   0.003,
	RerankerEstCost:      0.001,
}

// Override allows updating the global runtime config dynamically if needed.
func Override(custom Config) {
	Global = custom
}