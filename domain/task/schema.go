package task

// PlanSchema defines the strict JSON structure for the LLM response.
// This decouples the wire format from the internal Plan entity.
type PlanSchema struct {
	Reasoning string       `json:"reasoning"`
	Steps     []StepSchema `json:"steps"`
}

// StepSchema represents a single step in the JSON response.
type StepSchema struct {
	Description string   `json:"description"`
	Instruction string   `json:"instruction"`
	Files       []string `json:"files"`
}
