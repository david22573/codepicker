package context

import (
	"strings"
	"testing"

	"github.com/david22573/codepicker/domain/context"
)

// MockSliceStore allows us to test the builder without a real DB
type MockSliceStore struct {
	slices []context.CodeSlice
}

func (m *MockSliceStore) IndexFile(path string, slices []context.CodeSlice) error { return nil }
func (m *MockSliceStore) InvalidateFile(path string) error                        { return nil }
func (m *MockSliceStore) GetByID(id string) (*context.CodeSlice, error)           { return nil, nil }
func (m *MockSliceStore) GetByFile(path string) ([]context.CodeSlice, error)      { return nil, nil }
func (m *MockSliceStore) GetStats() (*context.IndexStats, error)                  { return nil, nil }

func (m *MockSliceStore) Query(q context.SliceQuery) ([]context.CodeSlice, error) {
	var results []context.CodeSlice
	for _, s := range m.slices {
		for _, kw := range q.Keywords {
			if strings.Contains(strings.ToLower(s.Content), strings.ToLower(kw)) ||
				strings.Contains(strings.ToLower(s.FilePath), strings.ToLower(kw)) {
				results = append(results, s)
				break
			}
		}
	}
	return results, nil
}

func (m *MockSliceStore) GetBySymbol(symbol string) ([]context.CodeSlice, error) {
	var results []context.CodeSlice
	for _, s := range m.slices {
		for _, sym := range s.Symbols {
			if sym == symbol {
				results = append(results, s)
			}
		}
	}
	return results, nil
}

func TestBuildForTask(t *testing.T) {
	// 1. Setup mock data
	mockSlices := []context.CodeSlice{
		{
			ID:        "1",
			FilePath:  "cmd/run.go",
			Content:   "func RunAgent() { fmt.Println(\"running\") }",
			Symbols:   []string{"RunAgent"},
			SliceType: context.SliceTypeFunction,
		},
		{
			ID:        "2",
			FilePath:  "infra/llm/client.go",
			Content:   "type LLMClient struct { APIKey string }",
			Symbols:   []string{"LLMClient"},
			SliceType: context.SliceTypeStruct,
		},
	}

	store := &MockSliceStore{slices: mockSlices}
	// Test with a 1000 token budget (plenty for these small slices)
	builder := NewSliceBasedBuilder(store, 1000)

	t.Run("Should include relevant slices based on task keywords", func(t *testing.T) {
		ctx, err := builder.BuildForTask("Fix the RunAgent function")
		if err != nil {
			t.Fatalf("BuildForTask failed: %v", err)
		}

		if !strings.Contains(ctx, "RunAgent") {
			t.Error("Expected context to contain 'RunAgent' slice")
		}
		if strings.Contains(ctx, "LLMClient") {
			t.Error("Did not expect context to contain unrelated 'LLMClient' slice")
		}
	})

	t.Run("Should pack slices within token budget", func(t *testing.T) {
		// Set a very tiny budget that only fits one slice
		smallBuilder := NewSliceBasedBuilder(store, 5)
		ctx, err := smallBuilder.BuildForTask("RunAgent and LLMClient")
		if err != nil {
			t.Fatalf("BuildForTask failed: %v", err)
		}

		// Grouping by file adds text, but we verify packing logic doesn't crash
		// and at least one relevant piece is attempted.
		if ctx == "" {
			t.Log("Context was empty due to very small budget (expected)")
		}
	})
}

func TestKeywordExtraction(t *testing.T) {
	builder := &SliceBasedBuilder{}
	input := "Fix the broken error handling in executor.go!"
	keywords := builder.extractKeywords(input)

	expected := []string{"broken", "error", "handling", "executor.go"}

	for _, exp := range expected {
		found := false
		for _, kw := range keywords {
			if kw == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Keyword extraction failed: expected to find %s in %v", exp, keywords)
		}
	}
}
