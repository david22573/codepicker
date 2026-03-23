package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/infra/llm"
	"github.com/david22573/codepicker/infra/validation"
)

type LearningStore interface {
	SaveLearning(ctx context.Context, l *agent.Learning) error
}

type RecordLearningTool struct {
	store    LearningStore
	embedder *llm.EmbeddingClient
}

func NewRecordLearningTool(store LearningStore, embedder *llm.EmbeddingClient) *RecordLearningTool {
	return &RecordLearningTool{store: store, embedder: embedder}
}

func (t *RecordLearningTool) Name() string { return "record_learning" }
func (t *RecordLearningTool) Description() string {
	return `Save a note or solution to long-term memory for future use. Use this when you figure out a tricky bug, undocumented setup step, or architectural rule.
Input: JSON with "task" (a brief summary of what you were trying to do) and "note" (the detailed solution or rule).`
}

func (t *RecordLearningTool) Execute(ctx context.Context, args string) (string, error) {
	var input struct {
		Task string `json:"task"`
		Note string `json:"note"`
	}

	if err := validation.DecodeStrict(args, &input); err != nil {
		return "", err
	}

	if input.Task == "" || input.Note == "" {
		return "", fmt.Errorf("validation error: missing 'task' or 'note'")
	}

	vectors, _, err := t.embedder.CreateEmbeddings(ctx, []string{input.Task + "\n" + input.Note})
	if err != nil {
		return "", fmt.Errorf("failed to generate embedding for learning: %w", err)
	}

	l := &agent.Learning{
		ID:        fmt.Sprintf("lrn_%d", time.Now().UnixMilli()),
		Task:      input.Task,
		Note:      input.Note,
		Embedding: vectors[0],
		CreatedAt: time.Now(),
	}

	if err := t.store.SaveLearning(ctx, l); err != nil {
		return "", fmt.Errorf("failed to save learning to database: %w", err)
	}

	return fmt.Sprintf("Successfully saved learning. Future tasks similar to '%s' will recall this note.", input.Task), nil
}
