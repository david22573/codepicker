package agent

import (
	"encoding/json"
	"regexp"
	"strings"
)

type ToolAction struct {
	Tool  string          `json:"tool"`
	Input json.RawMessage `json:"input"`
}

// ParseBatchActions extracts multiple tool calls from a single LLM response.
func ParseBatchActions(response string) (thought string, actions []ToolAction) {
	// Extract Thought
	thoughtRegex := regexp.MustCompile(`(?s)Thought:\s*(.*?)(?:\nAction:|Final Answer:|$)`)
	if match := thoughtRegex.FindStringSubmatch(response); len(match) > 1 {
		thought = strings.TrimSpace(match[1])
	}

	// 1. Try to parse as a JSON array of actions (Batch Format)
	actionRegex := regexp.MustCompile(`(?s)Actions:\s*(\[.*\])`)
	if match := actionRegex.FindStringSubmatch(response); len(match) > 1 {
		if err := json.Unmarshal([]byte(match[1]), &actions); err == nil {
			return thought, actions
		}
	}

	// 2. Fallback: Parse multiple Action/Input pairs
	pairRegex := regexp.MustCompile(`(?s)Action:\s*(\w+)\s*Input:\s*({.*?})`)
	matches := pairRegex.FindAllStringSubmatch(response, -1)
	for _, m := range matches {
		actions = append(actions, ToolAction{
			Tool:  strings.TrimSpace(m[1]),
			Input: json.RawMessage(m[2]),
		})
	}

	return thought, actions
}
