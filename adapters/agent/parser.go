package agent

import (
	"regexp"
	"strings"
)

// Regex to catch XML-style tool usage seen in logs (e.g., <invoke name="...">)
// Added (?s) to allow dot matching newlines for multi-line file content
var xmlToolRegex = regexp.MustCompile(`(?s)<invoke name="(.*?)">(.*?)</invoke>`)
var xmlArgRegex = regexp.MustCompile(`(?s)<parameter name="(.*?)">(.*?)</parameter>`)

// Strategy 3: Moonshot/Kimi Special Tokens
// Matches: <|tool_call_begin|> functions.list_files:0 <|tool_call_argument_begin|> {"path": "."} <|tool_call_end|>
var kimiToolRegex = regexp.MustCompile(`<\|tool_call_begin\|>\s*(?:functions\.)?([\w_]+)(?::\d+)?\s*<\|tool_call_argument_begin\|>\s*(\{.*?\})\s*<\|tool_call_end\|>`)

// Regex to identify code blocks for masking
var codeBlockRegex = regexp.MustCompile("(?s)```.*?```")

// parseReActResponse extracts structured components from various LLM output formats.
// CRITICAL FIX: It masks out code blocks first to prevent "hallucinated" tool calls
// appearing inside generated code snippets (e.g., if the agent writes a regex parser).
func parseReActResponse(resp string) (thought, tool, args string) {
	// 1. Safety Masking: Ignore anything inside ``` code blocks ```
	// We use the masked response to FIND the tool calls, ensuring we don't
	// accidentally parse code generation as tool execution.
	safeResp := maskCodeBlocks(resp)

	// STRATEGY 3: Check for Kimi/Moonshot Special Tokens
	// We check this first as it is the most specific and rigid format.
	if kimiToolRegex.MatchString(safeResp) {
		// We match against the original resp because we verified the tokens are NOT in a code block
		// via safeResp, and we need the actual content (which might contain code-like chars).
		// However, since we know the tokens exist outside blocks, finding them in resp is safe.
		matches := kimiToolRegex.FindStringSubmatch(resp)
		if len(matches) > 2 {
			tool = matches[1] // The function name (e.g. list_files)
			args = matches[2] // The JSON arguments

			// Capture thought: text appearing before the tool call
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
	// This takes priority if the model has drifted into XML mode.
	if xmlToolRegex.MatchString(safeResp) {
		// Again, we find the match indices in the SAFE string to ensure validity,
		// but we extract from the ORIGINAL string to get the content.
		loc := xmlToolRegex.FindStringIndex(safeResp)
		if loc != nil {
			// Extract the full matching string from the ORIGINAL response using the safe indices
			fullMatch := resp[loc[0]:loc[1]]
			matches := xmlToolRegex.FindStringSubmatch(fullMatch)

			if len(matches) > 1 {
				tool = matches[1]
				// Parse inner parameters into JSON-like string
				rawArgs := matches[2]
				argMatches := xmlArgRegex.FindAllStringSubmatch(rawArgs, -1)

				// Reconstruct simplistic JSON
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
					jsonBuilder.WriteString(escapeJSON(m[2])) // Escape quotes in value
					jsonBuilder.WriteString(`"`)
				}
				jsonBuilder.WriteString("}")
				args = jsonBuilder.String()

				// Thought is everything before the tag
				thought = strings.TrimSpace(resp[:loc[0]])
				return
			}
		}
	}

	// STRATEGY 2: Standard ReAct Parsing (Original Logic)
	// We run this line-by-line on the SAFE response to avoid picking up "Action:" inside code.
	lines := strings.Split(safeResp, "\n")

	// We need a map to correlate safe lines back to original lines to recover content
	// (e.g. if the user wrote "Action: write_file" we need the Input that follows).
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
			// We use the original line here because the input ITSELF might be code
			// and we want that content. We only masked the markers.
			inputBuilder.WriteString(originalLine + "\n")
			continue
		}

		// 2. Detect Keywords
		if strings.HasPrefix(cleanLine, "Thought:") {
			// Use original line to capture the full thought content
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
			// If model forgets "Thought:", treat early text as thought
			// Only if it's not a masked blank line
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

// maskCodeBlocks replaces content inside ```...``` with newlines to preserve
// line counts but hide keywords from the parser.
func maskCodeBlocks(input string) string {
	return codeBlockRegex.ReplaceAllStringFunc(input, func(match string) string {
		// Replace content with newlines to keep line numbers in sync
		// This ensures "Action:" on line 50 stays on line 50.
		newlines := strings.Count(match, "\n")
		return strings.Repeat("\n", newlines)
	})
}

// cleanInput removes markdown code blocks and extra whitespace
func cleanInput(raw string) string {
	raw = strings.TrimSpace(raw)
	// Strip markdown code blocks ```json ... ```
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
