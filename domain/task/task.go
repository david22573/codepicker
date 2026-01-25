package task

import "time"

type TaskStatus string

const (
	StatusPending   TaskStatus = "pending"
	StatusRunning   TaskStatus = "running"
	StatusCompleted TaskStatus = "completed"
	StatusFailed    TaskStatus = "failed"
)

type Step struct {
	ID          int
	Description string
	Instruction string
	Critical    bool
	Files       []string
	Status      string
	Result      string
}

type Plan struct {
	ID            string
	OriginalTask  string
	Steps         []Step
	EstimatedCost float64
	Reasoning     string
	Status        TaskStatus
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func NewPlan(id, task, reasoning string) *Plan {
	return &Plan{
		ID:           id,
		OriginalTask: task,
		Reasoning:    reasoning,
		Status:       StatusPending,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
}

func (p *Plan) AddStep(description, instruction string, critical bool, files []string) {
	step := Step{
		ID:          len(p.Steps) + 1,
		Description: description,
		Instruction: instruction,
		Critical:    critical,
		Files:       files,
		Status:      StatusPending,
	}
	p.Steps = append(p.Steps, step)
}
