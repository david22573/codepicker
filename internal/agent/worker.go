package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/prompts"
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
	w.Logger.Info(fmt.Sprintf("👷 Worker starting task using model: %s", w.Model))

	var fileContext strings.Builder
	for _, f := range files {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}

		content, err := w.FS.ReadFile(f)
		if err != nil {
			w.Logger.Warn(fmt.Sprintf("Worker could not read file %s: %v", f, err))
			continue
		}
		fileContext.WriteString(fmt.Sprintf("\n--- FILE: %s ---\n%s\n", f, string(content)))
	}

	// Format prompt for the worker
	workerPrompt := fmt.Sprintf(prompts.Worker, fileContext.String(), instruction)

	req := openrouter.ChatCompletionRequest{
		Model: w.Model, // STRICTLY use the assigned worker model
		Messages: []openrouter.ChatMessage{
			{Role: "system", Content: workerPrompt},
			{Role: "user", Content: "Execute the instruction. Return only the code changes or result."},
		},
		// We can add tools here if the worker needs them, but usually
		// the worker simply generates the text content for the shadow file tool
		// or returns text. If you want the worker to call tools, you can pass them here.
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
