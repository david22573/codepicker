package agent

import (
	"time"

	"github.com/david22573/codepicker/domain/event"
)

// EventEmitter centralizes all domain event publishing for the agent loop.
type EventEmitter struct {
	bus *event.DataBus
}

func NewEventEmitter(bus *event.DataBus) *EventEmitter {
	return &EventEmitter{bus: bus}
}

func (e *EventEmitter) publish(typ event.EventType, payload map[string]any) {
	if e.bus != nil {
		e.bus.Publish(event.Event{
			Type:      typ,
			Payload:   payload,
			Timestamp: time.Now().Unix(),
		})
	}
}

func (e *EventEmitter) Error(err error) {
	e.publish(event.EventError, map[string]any{"error": err.Error()})
}

func (e *EventEmitter) Cancelled() {
	e.publish(event.EventError, map[string]any{"error": "cancelled", "category": "Cancellation"})
}

func (e *EventEmitter) Thought(turn int, content string) {
	e.publish(event.EventAgentThought, map[string]any{"turn": turn, "content": content})
}

func (e *EventEmitter) Finish(result string) {
	e.publish(event.EventAgentFinish, map[string]any{"result": result})
}

func (e *EventEmitter) ToolStart(name, args string) {
	e.publish(event.EventToolStart, map[string]any{"tool": name, "input": args})
}

func (e *EventEmitter) ToolEnd(name, output string) {
	e.publish(event.EventToolEnd, map[string]any{"tool": name, "status": "finished", "output": output})
}

// --- Phase 4: Observability Expansions ---

func (e *EventEmitter) BudgetReserved(amount float64) {
	e.publish(event.EventBudgetReserve, map[string]any{"amount": amount})
}

func (e *EventEmitter) BudgetCommitted(amount float64) {
	e.publish(event.EventBudgetCommit, map[string]any{"amount": amount})
}

func (e *EventEmitter) BudgetExceeded() {
	e.publish(event.EventBudgetExhausted, map[string]any{"category": "BudgetExceeded"})
}

func (e *EventEmitter) TurnLimitReached(limit int) {
	e.publish(event.EventTurnLimitReached, map[string]any{"limit": limit, "category": "TurnLimitExceeded"})
}

func (e *EventEmitter) MemoryPruned(prunedCount int) {
	if prunedCount > 0 {
		e.publish(event.EventMemoryPruned, map[string]any{"pruned_messages_count": prunedCount})
	}
}

func (e *EventEmitter) PolicyBlocked(tool, reason string) {
	e.publish(event.EventPolicyBlock, map[string]any{"tool": tool, "reason": reason, "category": "PolicyBlocked"})
}

func (e *EventEmitter) ToolRetry(tool, reason string) {
	e.publish(event.EventToolRetry, map[string]any{"tool": tool, "reason": reason})
}