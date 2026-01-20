package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/vfs"
	"github.com/david22573/codepicker/pkg/openrouter"
)

type WorkerRunner struct {
	Client *openrouter.Client
	Model  string
	FS     vfs.VirtualFileSystem
	Logger logger.Logger
}

func NewWorkerRunner(client *openrouter.Client, model string, fs vfs.VirtualFileSystem, log logger.Logger) *WorkerRunner {
	return &WorkerRunner{
		Client: client,
		Model:  model,
		FS:     fs,
		Logger: log,
	}
}

func (w *WorkerRunner) Run(ctx context.Context, instruction string, files []string) (string, error) {
	var fileContext strings.Builder
	for _, f := range files {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}

		// Use VFS to ensure worker sees shadow changes
		content, err := w.FS.ReadFile(f)
		if err != nil {
			w.Logger.Warn(fmt.Sprintf("Worker could not read file %s: %v", f, err))
			continue
		}
		fileContext.WriteString(fmt.Sprintf("\n--- FILE: %s ---\n%s\n", f, string(content)))
	}

	workerPrompt := fmt.Sprintf(
		"You are a Worker Agent. You perform concrete tasks efficiently.\n"+
			"CONTEXT:\n%s\n"+
			"INSTRUCTION: %s\n"+
			"Output ONLY the result or code change. Do not chatter.",
		fileContext.String(), instruction,
	)

	req := openrouter.ChatCompletionRequest{
		Model: w.Model,
		Messages: []openrouter.ChatMessage{
			{Role: "system", Content: workerPrompt},
			{Role: "user", Content: "Execute the instruction."},
		},
	}

	resp, err := w.Client.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", err
	}

	if len(resp.Choices) > 0 && resp.Choices[0].Message != nil {
		return fmt.Sprintf("%v", resp.Choices[0].Message.Content), nil
	}
	return "No output from worker", nil
}
