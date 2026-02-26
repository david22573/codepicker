package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

// Transcript represents a serialized agent interaction session.
type Transcript struct {
	ID      string       `json:"id"`
	Model   string       `json:"model"`
	Records []TurnRecord `json:"records"`
}

// TurnRecord maps the input messages to the LLM's exact historical output.
type TurnRecord struct {
	TurnID        int       `json:"turn_id"`
	InputMessages []Message `json:"input_messages"`
	OutputMessage Message   `json:"output_message"`
	Cost          float64   `json:"cost"`
}

// ReplayAdapter acts as a drop-in LLM mock that strictly follows a pre-recorded transcript.
type ReplayAdapter struct {
	transcript Transcript
	turnIndex  int
}

// NewReplayAdapter loads a transcript file from disk and initializes the replay mock.
func NewReplayAdapter(filepath string) (*ReplayAdapter, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read transcript file: %w", err)
	}

	var t Transcript
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("failed to decode transcript JSON: %w", err)
	}

	return &ReplayAdapter{
		transcript: t,
		turnIndex:  0,
	}, nil
}

// ChatNative intercepts the standard LLM call and yields the recorded response.
func (r *ReplayAdapter) ChatNative(ctx context.Context, messages []Message, tools []ToolDefinition) (Message, []Message, error) {
	if r.turnIndex >= len(r.transcript.Records) {
		return Message{}, nil, fmt.Errorf("replay exhausted: no more records available in transcript")
	}

	record := r.transcript.Records[r.turnIndex]
	
	// Optional validation: could check if input `messages` closely match `record.InputMessages` 
	// to ensure the system hasn't drifted from the recorded deterministic path.

	r.turnIndex++
	return record.OutputMessage, nil, nil
}

// DumpTranscript provides an easy way to export an active agent's history to disk for future replays.
func DumpTranscript(id, model, filepath string, history []Message) error {
	var records []TurnRecord
	
	// Simplified extraction: assuming every assistant message was preceded by user/system/tool context
	turnCount := 0
	for i, msg := range history {
		if msg.Role == "assistant" {
			records = append(records, TurnRecord{
				TurnID:        turnCount,
				InputMessages: history[:i],
				OutputMessage: msg,
				Cost:          0.0, // Optionally calculate cost
			})
			turnCount++
		}
	}

	t := Transcript{
		ID:      id,
		Model:   model,
		Records: records,
	}

	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath, data, 0644)
}