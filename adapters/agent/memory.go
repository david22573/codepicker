package agent

import (
	"fmt"
	"strings"
)

// Turn represents a single cycle of reasoning and action.
type Turn struct {
	ID          int
	Thought     string
	Observation string // The output from the tool execution
}

// TurnMemory manages the sliding window of agent history to fit within context limits.
type TurnMemory struct {
	MaxChars int    // We use characters as a proxy for tokens (approx 4 chars = 1 token)
	Turns    []Turn // Chronological list of turns
}

// NewTurnMemory creates a memory buffer with a specific token limit.
func NewTurnMemory(maxTokens int) *TurnMemory {
	return &TurnMemory{
		// 1 Token ~= 4 Characters. We store the limit in chars for faster estimation.
		MaxChars: maxTokens * 4,
		Turns:    make([]Turn, 0),
	}
}

// Add appends a new turn and immediately enforces the size limit.
func (m *TurnMemory) Add(t Turn) {
	m.Turns = append(m.Turns, t)
	m.prune()
}

// prune removes the oldest turns until the total size fits within MaxChars.
func (m *TurnMemory) prune() {
	// We keep pruning while we are over budget, but we try to keep at least 1 turn
	// if it fits, or clear it if the single turn is massive (rare edge case).
	for m.EstimateSize() > m.MaxChars && len(m.Turns) > 0 {
		// Drop the oldest turn (FIFO)
		m.Turns = m.Turns[1:]
	}
}

// EstimateSize calculates the approximate character count of the rendered history.
func (m *TurnMemory) EstimateSize() int {
	size := 0
	for _, t := range m.Turns {
		// Add content length + generic formatting overhead
		size += len(t.Thought) + len(t.Observation) + 50
	}
	return size
}

// Render formats the stored turns into a string for the LLM prompt.
func (m *TurnMemory) Render() string {
	var sb strings.Builder
	for _, t := range m.Turns {
		sb.WriteString(fmt.Sprintf("\nThought: %s\nObservation: %s\n", t.Thought, t.Observation))
	}
	return sb.String()
}
