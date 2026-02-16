package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/infra/llm"
)

type SearchTool struct {
	Embedder *llm.EmbeddingClient
	Repo     agent.Repository
}

// NewSearchTool initializes the tool with the dependencies needed for semantic search.
func NewSearchTool(embedder *llm.EmbeddingClient, repo agent.Repository) *SearchTool {
	return &SearchTool{
		Embedder: embedder,
		Repo:     repo,
	}
}

func (t *SearchTool) Name() string {
	return "search_code"
}

func (t *SearchTool) Description() string {
	return `Search the codebase using natural language.
Input: A simple query string describing what you are looking for.
Example: "how is the database connection handled?"`
}

func (t *SearchTool) Execute(ctx context.Context, args string) (string, error) {
	// Cleanup input: strip surrounding quotes if the LLM adds them
	query := strings.TrimSpace(args)
	if len(query) > 1 && query[0] == '"' && query[len(query)-1] == '"' {
		query = query[1 : len(query)-1]
	}

	// 1. Convert the natural language query into a vector
	vector, err := t.Embedder.Embed(ctx, query)
	if err != nil {
		return "", fmt.Errorf("failed to generate embedding for query: %w", err)
	}

	// 2. Search the vector database for similar code chunks
	// We request the top 5 most relevant results
	results, err := t.Repo.VectorSearch(ctx, vector, 5)
	if err != nil {
		return "", fmt.Errorf("vector search failed: %w", err)
	}

	if len(results) == 0 {
		return "No relevant code found in the index.", nil
	}

	// 3. Format the results for the Agent
	var out strings.Builder
	out.WriteString(fmt.Sprintf("Found %d relevant code snippets:\n\n", len(results)))

	for _, r := range results {
		out.WriteString(fmt.Sprintf("--- FILE: %s ---\n", r.FilePath))
		out.WriteString(r.Content)
		out.WriteString("\n\n")
	}

	return out.String(), nil
}
