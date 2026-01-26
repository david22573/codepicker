package agent

import (
	"strings"
)

// parseReActResponse extracts the structured ReAct components from a raw string.
// It looks for "Thought:", "Action:", and "Input:" keywords.
func parseReActResponse(resp string) (thought, tool, args string) {
	lines := strings.Split(resp, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "Thought:") {
			// Extract everything after "Thought:"
			val := strings.TrimPrefix(line, "Thought:")
			thought = strings.TrimSpace(val)
		}

		if strings.HasPrefix(line, "Action:") {
			val := strings.TrimPrefix(line, "Action:")
			// Remove any markdown backticks if the LLM adds them
			val = strings.Trim(val, "` ")
			tool = strings.TrimSpace(val)
		}

		if strings.HasPrefix(line, "Input:") {
			val := strings.TrimPrefix(line, "Input:")
			// Remove any markdown backticks
			val = strings.Trim(val, "`")
			args = strings.TrimSpace(val)
		}
	}

	return
}
