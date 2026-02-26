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

func (e *EventEmitter) Error(err error) {
	if e.bus != nil {
		e.bus.Publish(event.Event{
			Type:    event.EventError,
			Payload: map[string]any{"error": err.Error()},
		})
	}
}

func (e *EventEmitter) Cancelled() {
	if e.bus != nil {
		e.bus.Publish(event.Event{
			Type:    event.EventError,
			Payload: map[string]any{"error": "cancelled"},
		})
	}
}

func (e *EventEmitter) BudgetExceeded() {
	if e.bus != nil {
		e.bus.Publish(event.Event{
			Type:    event.EventError,
			Payload: map[string]any{"error": "budget_exceeded"},
		})
	}
}

func (e *EventEmitter) Thought(turn int, content string) {
	if e.bus != nil {
		e.bus.Publish(event.Event{
			Type: event.EventAgentThought,
			Payload: map[string]any{
				"turn":    turn,
				"content": content,
			},
			Timestamp: time.Now().Unix(),
		})
	}
}

func (e *EventEmitter) Finish(result string) {
	if e.bus != nil {
		e.bus.Publish(event.Event{
			Type:    event.EventAgentFinish,
			Payload: map[string]any{"result": result},
		})
	}
}

func (e *EventEmitter) ToolStart(name, args string) {
	if e.bus != nil {
		e.bus.Publish(event.Event{
			Type:    event.EventToolStart,
			Payload: map[string]any{"tool": name, "input": args},
		})
	}
}

func (e *EventEmitter) ToolEnd(name, output string) {
	if e.bus != nil {
		e.bus.Publish(event.Event{
			Type:    event.EventToolEnd,
			Payload: map[string]any{"tool": name, "status": "finished", "output": output},
		})
	}
}