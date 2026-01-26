package agent

import (
	"time"

	"github.com/david22573/codepicker/domain/task"
)

// Execution represents a running session of an agent
type Execution struct {
	ID        string
	PlanID    string
	Status    task.Status
	History   []Interaction
	StartTime time.Time
	EndTime   time.Time
}

// Interaction records a single "turn" in the agent loop
type Interaction struct {
	TurnID    int
	Thought   string // The reasoning provided by the LLM
	ToolName  string // The tool the LLM chose
	ToolArgs  string // The arguments passed
	ToolOut   string // The result from the tool execution
	Timestamp time.Time
}

// NewExecution creates a new execution context
func NewExecution(id, planID string) *Execution {
	return &Execution{
		ID:        id,
		PlanID:    planID,
		Status:    task.StatusRunning,
		StartTime: time.Now(),
		History:   make([]Interaction, 0),
	}
}

// RecordTurn adds an interaction to the history
func (e *Execution) RecordTurn(thought, tool, args, output string) {
	e.History = append(e.History, Interaction{
		TurnID:    len(e.History) + 1,
		Thought:   thought,
		ToolName:  tool,
		ToolArgs:  args,
		ToolOut:   output,
		Timestamp: time.Now(),
	})
}

func (e *Execution) Finish() {
	e.Status = task.StatusCompleted
	e.EndTime = time.Now()
}

func (e *Execution) Fail() {
	e.Status = task.StatusFailed
	e.EndTime = time.Now()
}
