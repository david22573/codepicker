package agent

import (
	"strings"
)

// parseReActResponse extracts the structured ReAct components.
// It is robust against multi-line JSON and markdown artifacts.
func parseReActResponse(resp string) (thought, tool, args string) {
	lines := strings.Split(resp, "\n")

	// State flags
	inInput := false
	var inputBuilder strings.Builder

	for _, line := range lines {
		cleanLine := strings.TrimSpace(line)

		// 1. Capture Input (Multi-line handling)
		if inInput {
			inputBuilder.WriteString(line + "\n")
			continue
		}

		// 2. Detect Keywords
		if strings.HasPrefix(cleanLine, "Thought:") {
			val := strings.TrimPrefix(cleanLine, "Thought:")
			thought = strings.TrimSpace(val)
		} else if strings.HasPrefix(cleanLine, "Action:") {
			val := strings.TrimPrefix(cleanLine, "Action:")
			// Remove backticks if model adds them (e.g. `read_file`)
			val = strings.Trim(val, "` ")
			tool = strings.TrimSpace(val)
		} else if strings.HasPrefix(cleanLine, "Input:") {
			val := strings.TrimPrefix(cleanLine, "Input:")
			inInput = true
			inputBuilder.WriteString(val + "\n")
		} else if !inInput && thought == "" && tool == "" {
			// If model forgets "Thought:", treat early text as thought
			thought = cleanLine
		}
	}

	args = cleanInput(inputBuilder.String())
	return
}

// cleanInput removes markdown code blocks and extra whitespace
func cleanInput(raw string) string {
	raw = strings.TrimSpace(raw)
	// Strip markdown code blocks ```json ... ```
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	return strings.TrimSpace(raw)
}
