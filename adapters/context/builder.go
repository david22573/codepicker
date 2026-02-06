package context

import (
	"fmt"
	"sort"
	"strings"

	"github.com/david22573/codepicker/domain/context"
)

// SliceBasedBuilder finds the most relevant code chunks for a specific prompt
type SliceBasedBuilder struct {
	store     context.SliceStore
	maxTokens int
}

// NewSliceBasedBuilder initializes the builder with a persistence store and token limit
func NewSliceBasedBuilder(store context.SliceStore, maxTokens int) *SliceBasedBuilder {
	return &SliceBasedBuilder{
		store:     store,
		maxTokens: maxTokens,
	}
}

// BuildForTask extracts keywords from the task and retrieves relevant slices from the store
func (b *SliceBasedBuilder) BuildForTask(taskDescription string) (string, error) {
	keywords := b.extractKeywords(taskDescription)

	// Increased MaxResults to give the ranker more to work with
	query := context.SliceQuery{
		Keywords:   keywords,
		MaxResults: 60,
	}

	slices, err := b.store.Query(query)
	if err != nil {
		return "", fmt.Errorf("failed to query slices: %w", err)
	}

	ranked := b.rankSlices(slices, keywords)
	selected := b.packSlices(ranked, b.maxTokens)

	return b.formatContext(selected), nil
}

// rankSlices scores code chunks based on keyword matches in symbols and content
func (b *SliceBasedBuilder) rankSlices(slices []context.CodeSlice, keywords []string) []context.CodeSlice {
	type scoredSlice struct {
		slice context.CodeSlice
		score int
	}

	scored := make([]scoredSlice, len(slices))
	for i, s := range slices {
		score := 0
		for _, kw := range keywords {
			lowerKw := strings.ToLower(kw)

			// Symbols (func/struct names) are highest priority
			for _, sym := range s.Symbols {
				if strings.Contains(strings.ToLower(sym), lowerKw) {
					score += 20
				}
			}

			// Content matches
			if strings.Contains(strings.ToLower(s.Content), lowerKw) {
				score += 5
			}

			// File path matches
			if strings.Contains(strings.ToLower(s.FilePath), lowerKw) {
				score += 10
			}
		}
		scored[i] = scoredSlice{s, score}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	result := make([]context.CodeSlice, len(scored))
	for i, s := range scored {
		result[i] = s.slice
	}
	return result
}

// packSlices fits as many high-scoring slices as possible into the token limit
func (b *SliceBasedBuilder) packSlices(slices []context.CodeSlice, maxTokens int) []context.CodeSlice {
	var selected []context.CodeSlice
	totalTokens := 0

	for _, s := range slices {
		// More conservative estimation: ~3 characters per token for code
		estTokens := len(s.Content) / 3
		if totalTokens+estTokens > maxTokens {
			continue
		}
		selected = append(selected, s)
		totalTokens += estTokens
	}

	return selected
}

// formatContext renders the selected slices into a single Markdown block
func (b *SliceBasedBuilder) formatContext(slices []context.CodeSlice) string {
	var sb strings.Builder
	sb.WriteString("# RELEVANT CODE CONTEXT\n")
	sb.WriteString("The following code units were selected based on your current task.\n\n")

	byFile := make(map[string][]context.CodeSlice)
	for _, s := range slices {
		byFile[s.FilePath] = append(byFile[s.FilePath], s)
	}

	for path, fileSlices := range byFile {
		sb.WriteString(fmt.Sprintf("## File: %s\n", path))
		for _, s := range fileSlices {
			sb.WriteString(fmt.Sprintf("### %s (Lines %d-%d)\n", s.SliceType, s.StartLine, s.EndLine))
			sb.WriteString("```go\n")
			sb.WriteString(s.Content)
			sb.WriteString("\n```\n")
		}
		sb.WriteString("\n---\n")
	}

	return sb.String()
}

// extractKeywords cleans the task input for better search matching
func (b *SliceBasedBuilder) extractKeywords(text string) []string {
	stopWords := map[string]bool{"the": true, "for": true, "fix": true, "add": true, "and": true, "with": true, "how": true}
	words := strings.Fields(strings.ToLower(text))
	var keywords []string
	for _, w := range words {
		w = strings.Trim(w, ".,!?;:\"'")
		if len(w) > 2 && !stopWords[w] {
			keywords = append(keywords, w)
		}
	}
	return keywords
}
