package task

// CheckStatus defines the status of a single check
type CheckStatus string

const (
	CheckPass CheckStatus = "pass"
	CheckFail CheckStatus = "fail"
	CheckSkip CheckStatus = "skip"
)

// CheckResult holds the outcome of a single verification check
type CheckResult struct {
	Name       string      `json:"name"`
	Command    string      `json:"command,omitempty"`
	Status     CheckStatus `json:"status"`
	ExitCode   int         `json:"exit_code,omitempty"`
	DurationMS int64       `json:"duration_ms"`
	Stdout     string      `json:"stdout,omitempty"`
	Stderr     string      `json:"stderr,omitempty"`
	Error      string      `json:"error,omitempty"`
}

// CheckReport holds the collection of check results
type CheckReport struct {
	Status string        `json:"status"` // pass, fail
	Checks []CheckResult `json:"checks"`
}

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
