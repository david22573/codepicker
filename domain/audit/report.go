package audit

import (
	"time"
)

// Report represents the output of an audit session
type Report struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Model     string    `json:"model"`
	Content   string    `json:"content"`  // The Markdown analysis
	Artifact  string    `json:"artifact"` // Path to saved file
}
