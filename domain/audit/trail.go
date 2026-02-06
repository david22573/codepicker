package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// AuditEvent represents a single significant action taken by the system.
type AuditEvent struct {
	Type      string                 `json:"type"` // e.g., "plan_created", "tool_exec", "file_mod"
	Timestamp time.Time              `json:"timestamp"`
	Details   map[string]interface{} `json:"details"`
}

// AuditTrail maintains a chronological log of an execution session.
type AuditTrail struct {
	ExecutionID string       `json:"execution_id"`
	StartTime   time.Time    `json:"start_time"`
	Events      []AuditEvent `json:"events"`
	mu          sync.RWMutex
}

// NewAuditTrail creates a new tracker for a specific session.
func NewAuditTrail(execID string) *AuditTrail {
	return &AuditTrail{
		ExecutionID: execID,
		StartTime:   time.Now(),
		Events:      make([]AuditEvent, 0),
	}
}

// Record appends a new event to the trail in a thread-safe manner.
func (at *AuditTrail) Record(eventType string, details map[string]interface{}) {
	at.mu.Lock()
	defer at.mu.Unlock()

	event := AuditEvent{
		Type:      eventType,
		Timestamp: time.Now(),
		Details:   details,
	}
	at.Events = append(at.Events, event)
}

// Save writes the entire audit log to a JSON file at the specified path.
func (at *AuditTrail) Save(filePath string) error {
	at.mu.RLock()
	defer at.mu.RUnlock()

	data, err := json.MarshalIndent(at, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal audit trail: %w", err)
	}

	return os.WriteFile(filePath, data, 0644)
}
