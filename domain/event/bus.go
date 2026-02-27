package event

import (
	"sync"
)

// EventType defines the categories of agent activity.
type EventType string

const (
	EventAgentThought     EventType = "agent.thought"
	EventToolStart        EventType = "tool.start"
	EventToolEnd          EventType = "tool.end"
	EventAgentFinish      EventType = "agent.finish"
	EventError            EventType = "error"
	
	// Phase 4 & 5: Observability Expansion
	EventBudgetReserve    EventType = "budget.reserve"
	EventBudgetCommit     EventType = "budget.commit"
	EventBudgetExhausted  EventType = "budget.exhausted"
	EventTurnLimitReached EventType = "agent.turn_limit_reached"
	EventMemoryPruned     EventType = "memory.pruned"
	EventPolicyBlock      EventType = "policy.block"
	EventToolRetry        EventType = "tool.retry"
	EventSessionCost      EventType = "session.cost_update"
)

// Event represents a single unit of activity in the system.
type Event struct {
	Type      EventType              `json:"type"`
	Payload   map[string]interface{} `json:"payload"`
	Timestamp int64                  `json:"timestamp"`
}

// DataBus handles pub/sub for agent events.
type DataBus struct {
	mu          sync.RWMutex
	subscribers []chan Event
	closed      bool
}

func NewDataBus() *DataBus {
	return &DataBus{
		subscribers: make([]chan Event, 0),
	}
}

// Subscribe returns a channel that receives every event published.
func (b *DataBus) Subscribe() chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan Event, 100)
	b.subscribers = append(b.subscribers, ch)
	return ch
}

// Publish broadcasts an event to all active subscribers.
func (b *DataBus) Publish(e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return
	}

	for _, ch := range b.subscribers {
		select {
		case ch <- e:
		default:
			// Buffer full, skip to prevent blocking the agent loop
		}
	}
}

func (b *DataBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for _, ch := range b.subscribers {
		close(ch)
	}
}