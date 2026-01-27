package task

// CIResult defines the schema for machine-readable output
type CIResult struct {
	Status      string   `json:"status"` // success, failure
	Task        string   `json:"task"`
	PlanSummary string   `json:"plan_summary"`
	StepsTotal  int      `json:"steps_total"`
	StepsFailed int      `json:"steps_failed"`
	Artifacts   []string `json:"artifacts,omitempty"` // Generated files (shadow paths)
	Error       string   `json:"error,omitempty"`
}
