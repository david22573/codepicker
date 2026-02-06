package agent

import (
	"regexp"
	"strings"
)

// Regex to catch XML-style tool usage seen in logs (e.g., <invoke name="...">)
var xmlToolRegex = regexp.MustCompile(`<invoke name="(.*?)">(.*?)</invoke>`)
var xmlArgRegex = regexp.MustCompile(`<parameter name="(.*?)">(.*?)</parameter>`)

// parseReActResponse extracts structured components from various LLM output formats.
func parseReActResponse(resp string) (thought, tool, args string) {
	// STRATEGY 1: Check for XML format
	// This takes priority if the model has drifted into XML mode.
	if xmlToolRegex.MatchString(resp) {
		matches := xmlToolRegex.FindStringSubmatch(resp)
		if len(matches) > 1 {
			tool = matches[1]
			// Parse inner parameters into JSON-like string for the tool executor
			rawArgs := matches[2]
			argMatches := xmlArgRegex.FindAllStringSubmatch(rawArgs, -1)

			// Reconstruct simplistic JSON for the existing tool interface
			jsonBuilder := new(strings.Builder)
			jsonBuilder.WriteString("{")
			for i, m := range argMatches {
				if i > 0 {
					jsonBuilder.WriteString(", ")
				}
				// key: m[1], value: m[2]
				jsonBuilder.WriteString(`"`)
				jsonBuilder.WriteString(m[1])
				jsonBuilder.WriteString(`": "`)
				jsonBuilder.WriteString(m[2])
				jsonBuilder.WriteString(`"`)
			}
			jsonBuilder.WriteString("}")
			args = jsonBuilder.String()

			// Everything before the tag is considered thought
			loc := xmlToolRegex.FindStringIndex(resp)
			if loc != nil {
				thought = strings.TrimSpace(resp[:loc[0]])
			}
			return
		}
	}

	// STRATEGY 2: Standard ReAct Parsing (Original Logic)
	lines := strings.Split(resp, "\n")
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

	if tool != "" {
		args = cleanInput(inputBuilder.String())
	}
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
