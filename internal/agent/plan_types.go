package agent

type Plan struct {
	ID            string  `json:"id"`
	OriginalTask  string  `json:"original_task"`
	Steps         []Step  `json:"steps"`
	EstimatedCost float64 `json:"estimated_cost_usd"`
	Reasoning     string  `json:"reasoning"`
}

type Step struct {
	ID          int      `json:"id"`
	Description string   `json:"description"`
	Instruction string   `json:"instruction"`     // The prompt to give the worker agent
	Critical    bool     `json:"critical"`        // If true, stop on failure
	Files       []string `json:"files,omitempty"` // Files relevant to this step
	Status      string   `json:"status"`          // pending, running, completed, failed
	Result      string   `json:"result,omitempty"`
}

// AIPlanResponse matches the JSON schema expected from the LLM
type AIPlanResponse struct {
	Steps     []Step  `json:"steps"`
	Reasoning string  `json:"reasoning"`
	CostEst   float64 `json:"estimated_cost"`
}
