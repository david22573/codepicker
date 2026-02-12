package agent

import (
	"fmt"
)

type ObservationProcessor struct {
	maxChars int
}

func NewObservationProcessor(maxChars int) *ObservationProcessor {
	return &ObservationProcessor{maxChars: maxChars}
}

// Process sanitizes and truncates tool output for the next turn.
func (p *ObservationProcessor) Process(output string) string {
	if len(output) <= p.maxChars {
		return output
	}

	// Strategy: Keep first 20% and last 70% of the output (middle is usually less relevant)
	headLimit := int(float64(p.maxChars) * 0.2)
	tailLimit := int(float64(p.maxChars) * 0.7)

	head := output[:headLimit]
	tail := output[len(output)-tailLimit:]

	return fmt.Sprintf("%s\n\n... [TRUNCATED %d characters] ...\n\n%s",
		head, len(output)-(headLimit+tailLimit), tail)
}

// FormatForKimi adds specific cues that help K2.5 distinguish system vs tool output.
func (p *ObservationProcessor) FormatForKimi(toolName, output string) string {
	processed := p.Process(output)
	return fmt.Sprintf("[TOOL_RESULT: %s]\n%s\n[END_RESULT]", toolName, processed)
}
