package agent

import (
	"time"

	"github.com/david22573/codepicker/domain/task"
)

// Execution represents a single run of an agent working on a plan
type Execution struct {
	ID        string
	PlanID    string
	Status    string
	History   []Interaction
	Variables map[string]string
	StartTime time.Time
	EndTime   time.Time
}

// Interaction records a single turn (Thought -> Tool -> Result)
type Interaction struct {
	StepID    int
	Thought   string
	ToolName  string
	ToolArgs  string
	ToolOut   string
	Timestamp time.Time
}

func NewExecution(id, planID string) *Execution {
	return &Execution{
		ID:        id,
		PlanID:    planID,
		Status:    task.StatusRunning,
		Variables: make(map[string]string),
		StartTime: time.Now(),
	}
}

func (e *Execution) RecordTurn(stepID int, thought, tool, args, output string) {
	e.History = append(e.History, Interaction{
		StepID:    stepID,
		Thought:   thought,
		ToolName:  tool,
		ToolArgs:  args,
		ToolOut:   output,
		Timestamp: time.Now(),
	})
}

func (e *Execution) Complete() {
	e.Status = task.StatusCompleted
	e.EndTime = time.Now()
}

func (e *Execution) Fail() {
	e.Status = task.StatusFailed
	e.EndTime = time.Now()
}
