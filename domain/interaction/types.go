package interaction

// Analysis holds the insights gathered during the read-only phase
// of the Two-Pass workflow.
type Analysis struct {
	// Markdown is the natural language report/diagnosis from the agent.
	Markdown string `json:"markdown"`

	// Files is the list of files identified as relevant to the task.
	Files []string `json:"files"`
}

// Patch holds the generated code fix.
type Patch struct {
	// Diff contains the raw Git Unified Diff string.
	Diff string `json:"diff"`
}
