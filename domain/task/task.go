package task

import (
	"time"

	"github.com/david22573/codepicker/domain/errors"
)

// Status represents the lifecycle state of a task or step
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

// Step represents a single unit of work in a plan
type Step struct {
	ID          int      `json:"id"`
	Description string   `json:"description"` // Human-readable goal
	Instruction string   `json:"instruction"` // Prompt for the worker agent
	Files       []string `json:"files"`       // Context files needed
	Status      Status   `json:"status"`
	Result      string   `json:"result,omitempty"`
	Error       error    `json:"-"`
}

// Plan represents a sequence of steps to achieve a goal
type Plan struct {
	ID            string    `json:"id"`
	OriginalTask  string    `json:"original_task"`
	Reasoning     string    `json:"reasoning"`
	Steps         []Step    `json:"steps"`
	EstimatedCost float64   `json:"estimated_cost"`
	Status        Status    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
}

// NewPlan creates a fresh plan
func NewPlan(id, taskStr, reasoning string) *Plan {
	return &Plan{
		ID:           id,
		OriginalTask: taskStr,
		Reasoning:    reasoning,
		Status:       StatusPending,
		CreatedAt:    time.Now(),
		Steps:        make([]Step, 0),
	}
}

// AddStep appends a step to the plan
func (p *Plan) AddStep(description, instruction string, files []string) {
	p.Steps = append(p.Steps, Step{
		ID:          len(p.Steps) + 1,
		Description: description,
		Instruction: instruction,
		Files:       files,
		Status:      StatusPending,
	})
}

// MarkStepComplete updates a step's status
func (p *Plan) MarkStepComplete(stepID int, result string) error {
	if stepID < 1 || stepID > len(p.Steps) {
		return errors.NewValidation("plan.MarkStepComplete", "invalid step ID")
	}
	// Steps are 1-indexed for display, 0-indexed for slice
	p.Steps[stepID-1].Status = StatusCompleted
	p.Steps[stepID-1].Result = result
	return nil
}

// MarkStepFailed updates a step's error state
func (p *Plan) MarkStepFailed(stepID int, err error) error {
	if stepID < 1 || stepID > len(p.Steps) {
		return errors.NewValidation("plan.MarkStepFailed", "invalid step ID")
	}
	p.Steps[stepID-1].Status = StatusFailed
	p.Steps[stepID-1].Error = err
	return nil
}
