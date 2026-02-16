package agent

import (
	"regexp"
	"strings"
)

// Regex to catch XML-style tool usage seen in logs (e.g., <invoke name="...">)
var xmlToolRegex = regexp.MustCompile(`(?s)<invoke name="(.*?)">(.*?)</invoke>`)
var xmlArgRegex = regexp.MustCompile(`(?s)<parameter name="(.*?)">(.*?)</parameter>`)

// Strategy 3: Moonshot/Kimi Special Tokens
var kimiToolRegex = regexp.MustCompile(`<\|tool_call_begin\|>\s*(?:functions\.)?([\w_]+)(?::\d+)?\s*<\|tool_call_argument_begin\|>\s*(\{.*?\})\s*<\|tool_call_end\|>`)

// Regex to identify code blocks for masking
var codeBlockRegex = regexp.MustCompile("(?s)```.*?```")

// parseReActResponse extracts structured components from various LLM output formats.
func parseReActResponse(resp string) (thought, tool, args string) {
	// 1. Safety Masking: Ignore anything inside ``` code blocks ```
	safeResp := maskCodeBlocks(resp)

	// STRATEGY 3: Check for Kimi/Moonshot Special Tokens
	if kimiToolRegex.MatchString(safeResp) {
		matches := kimiToolRegex.FindStringSubmatch(resp)
		if len(matches) > 2 {
			tool = matches[1]
			args = matches[2]
			loc := kimiToolRegex.FindStringIndex(resp)
			if loc != nil {
				thought = strings.TrimSpace(resp[:loc[0]])
				thought = strings.ReplaceAll(thought, "<|tool_calls_section_begin|>", "")
				thought = strings.TrimSpace(thought)
			}
			return
		}
	}

	// STRATEGY 1: Check for XML format
	if xmlToolRegex.MatchString(safeResp) {
		loc := xmlToolRegex.FindStringIndex(safeResp)
		if loc != nil {
			fullMatch := resp[loc[0]:loc[1]]
			matches := xmlToolRegex.FindStringSubmatch(fullMatch)

			if len(matches) > 1 {
				tool = matches[1]
				rawArgs := matches[2]
				argMatches := xmlArgRegex.FindAllStringSubmatch(rawArgs, -1)

				jsonBuilder := new(strings.Builder)
				jsonBuilder.WriteString("{")
				for i, m := range argMatches {
					if i > 0 {
						jsonBuilder.WriteString(", ")
					}
					jsonBuilder.WriteString(`"`)
					jsonBuilder.WriteString(m[1])
					jsonBuilder.WriteString(`": "`)
					jsonBuilder.WriteString(escapeJSON(m[2]))
					jsonBuilder.WriteString(`"`)
				}
				jsonBuilder.WriteString("}")
				args = jsonBuilder.String()
				thought = strings.TrimSpace(resp[:loc[0]])
				return
			}
		}
	}

	// STRATEGY 2: Standard ReAct Parsing (Original Logic)
	lines := strings.Split(safeResp, "\n")
	originalLines := strings.Split(resp, "\n")

	inInput := false
	var inputBuilder strings.Builder

	for i, line := range lines {
		if i >= len(originalLines) {
			break
		}

		cleanLine := strings.TrimSpace(line)
		originalLine := originalLines[i]

		// 1. Capture Input (Multi-line handling)
		if inInput {
			// FIX: Stop capturing if we see a new keyword
			if strings.HasPrefix(cleanLine, "Thought:") ||
				strings.HasPrefix(cleanLine, "Action:") ||
				strings.HasPrefix(cleanLine, "Final Answer:") {
				inInput = false
				// Fallthrough to process this line as a keyword
			} else {
				// Continue capturing input
				inputBuilder.WriteString(originalLine + "\n")
				continue
			}
		}

		// 2. Detect Keywords
		if strings.HasPrefix(cleanLine, "Thought:") {
			val := strings.TrimPrefix(originalLine, "Thought:")
			thought = strings.TrimSpace(val)
		} else if strings.HasPrefix(cleanLine, "Action:") {
			val := strings.TrimPrefix(originalLine, "Action:")
			val = strings.Trim(val, "` ")
			tool = strings.TrimSpace(val)
		} else if strings.HasPrefix(cleanLine, "Input:") {
			val := strings.TrimPrefix(originalLine, "Input:")
			inInput = true
			inputBuilder.WriteString(val + "\n")
		} else if !inInput && thought == "" && tool == "" {
			if strings.TrimSpace(cleanLine) != "" {
				thought = strings.TrimSpace(originalLine)
			}
		}
	}

	if tool != "" {
		args = cleanInput(inputBuilder.String())
	}
	return
}

// maskCodeBlocks replaces content inside ```...``` with newlines
func maskCodeBlocks(input string) string {
	return codeBlockRegex.ReplaceAllStringFunc(input, func(match string) string {
		newlines := strings.Count(match, "\n")
		return strings.Repeat("\n", newlines)
	})
}

// cleanInput removes markdown code blocks and extra whitespace
func cleanInput(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```json") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimSuffix(raw, "```")
	} else if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```")
		raw = strings.TrimSuffix(raw, "```")
	}
	return strings.TrimSpace(raw)
}

func escapeJSON(val string) string {
	val = strings.ReplaceAll(val, `\`, `\\`)
	val = strings.ReplaceAll(val, `"`, `\"`)
	val = strings.ReplaceAll(val, "\n", `\n`)
	return val
}
