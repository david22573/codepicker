package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
)

// StreamRenderer handles accumulating tokens and rendering Markdown content
// neatly to the terminal without "flickering".
type StreamRenderer struct {
	renderer *glamour.TermRenderer
	buffer   strings.Builder
	printed  int
}

func NewStreamRenderer() (*StreamRenderer, error) {
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(100),
	)
	if err != nil {
		return nil, err
	}

	return &StreamRenderer{
		renderer: r,
	}, nil
}

// StreamToken adds a chunk of text to the buffer.
// It does NOT render immediately to avoid breaking ansi codes in partial renders.
// It simply prints the raw text for immediate feedback, or you can accumulate
// and render chunks if you prefer a "replace" strategy.
//
// For this CLI, we will stick to a simple "append" strategy for tokens,
// and a "final" render for blocks.
func (s *StreamRenderer) StreamToken(token string) {
	fmt.Print(token)
	s.buffer.WriteString(token)
}

// RenderFinal flushes the buffer and renders it as styled Markdown.
// Call this when the message is complete.
func (s *StreamRenderer) RenderFinal() {
	raw := s.buffer.String()

	// Clear the raw output we just streamed to replace it with the pretty version
	// Note: This simple approach assumes we can clear line-by-line.
	// For complex CLIs, we might just print a newline separator instead.
	fmt.Println()
	fmt.Println(strings.Repeat("─", 40))

	out, err := s.renderer.Render(raw)
	if err != nil {
		fmt.Println(raw) // Fallback
	} else {
		fmt.Print(out)
	}

	// Reset for next turn
	s.buffer.Reset()
}

// RenderBlock renders a static block of markdown immediately.
func RenderMarkdown(content string) {
	r, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(100),
	)
	out, _ := r.Render(content)
	fmt.Print(out)
}
