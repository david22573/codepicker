package agent

import (
	"encoding/json"
	"testing"
)

// FuzzParseBatchActions ensures the batch parser never crashes on malformed LLM output.
func FuzzParseBatchActions(f *testing.F) {
	// 1. Seed the corpus with known valid patterns
	f.Add(`Thought: I will list files.
Actions: [{"tool": "list_files", "input": {"path": "."}}]`)

	f.Add(`Thought: Checking main.go
Action: read_file
Input: {"path": "main.go"}`)

	f.Add(`Just some random text with no actions.`)
	f.Add(`Action: incomplete_tool Input: {`)

	// 2. Fuzzing Loop
	f.Fuzz(func(t *testing.T, input string) {
		// Execute the function under test
		thought, actions := ParseBatchActions(input)

		// 3. Invariants / Assertions
		// Invariant A: Thought is never nil (though it can be empty)
		if len(thought) > len(input) {
			t.Errorf("Thought cannot be longer than input")
		}

		// Invariant B: If actions exist, they must be safe to read
		for _, action := range actions {
			if action.Tool == "" {
				// While technically allowed by loosely parsed regex, mostly we want to ensure
				// we don't have partial/corrupted structs causing issues downstream.
				// For fuzzing, main goal is "Did we panic?" (Go handles that implicitly).
			}

			// Ensure Input is valid JSON bytes if present
			if len(action.Input) > 0 {
				var js interface{}
				// We don't fail the test if invalid JSON is returned (garbage in, garbage out),
				// but we want to ensure the bytes themselves are safe to access.
				_ = json.Unmarshal(action.Input, &js)
			}
		}
	})
}

// FuzzParseReActResponse ensures the legacy ReAct parser is robust.
func FuzzParseReActResponse(f *testing.F) {
	// 1. Seed corpus
	f.Add(`Thought: thinking...
Action: run_cmd
Input: {"command": "ls"}`)

	f.Add(`<invoke name="read_file"><parameter name="path">main.go</parameter></invoke>`)

	f.Add(`<|tool_call_begin|>list_files<|tool_call_argument_begin|>{"path":"."}<|tool_call_end|>`)

	// 2. Fuzzing Loop
	f.Fuzz(func(t *testing.T, input string) {
		thought, tool, args := parseReActResponse(input)

		// Assertions: basic sanity checks
		if len(tool) > 0 && len(args) == 0 {
			// It's acceptable to have a tool without args, but let's just ensure
			// we didn't crash extracting them.
		}

		// Check for excessive allocations or runaway string building
		if len(thought)+len(tool)+len(args) > len(input)*2+100 {
			t.Errorf("Output expansion detected: possible memory issues")
		}
	})
}
