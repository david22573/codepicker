package batch

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/david22573/codepicker/internal/app"
	"github.com/david22573/codepicker/internal/database"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/policy"
	"github.com/david22573/codepicker/pkg/openrouter"
)

type Runner struct {
	Queue       *Queue
	Store       *database.Store
	Logger      logger.Logger
	Concurrency int
	SrcDir      string
	// Removed Client/APIKey fields - AgentContext handles this now
}

func NewRunner(q *Queue, s *database.Store, log logger.Logger, workers int, srcDir string) *Runner {
	if workers < 1 {
		workers = 1
	}
	return &Runner{
		Queue:       q,
		Store:       s,
		Logger:      log,
		Concurrency: workers,
		SrcDir:      srcDir,
	}
}

func (r *Runner) Start(ctx context.Context) error {
	r.Logger.Info(fmt.Sprintf("🚀 Starting Batch Runner with %d workers", r.Concurrency))
	r.Logger.Info("🛡️  Policy: Batch (No shell access, strict allowances)")

	var wg sync.WaitGroup
	sem := make(chan struct{}, r.Concurrency)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.Logger.Info("🛑 Batch Runner stopping...")
			wg.Wait()
			return nil
		case <-ticker.C:
			select {
			case sem <- struct{}{}:
				// Try to claim a job
				job, err := r.Queue.Next()
				if err != nil {
					r.Logger.Error("Queue error: " + err.Error())
					<-sem
					continue
				}
				if job == nil {
					<-sem // Release slot if no job
					continue
				}

				wg.Add(1)
				go func(j *Job) {
					defer wg.Done()
					defer func() { <-sem }()
					r.processJob(ctx, j)
				}(job)
			default:
				continue
			}
		}
	}
}

func (r *Runner) processJob(ctx context.Context, job *Job) {
	r.Logger.Info(fmt.Sprintf("👷 Picking up Job %s: %s", job.ID[:8], job.Task))

	if err := r.Queue.UpdateStatus(job.ID, StatusRunning, "", ""); err != nil {
		r.Logger.Error("Failed to update status: " + err.Error())
		return
	}

	// 1. Initialize Safe Context per Job
	// We use ModeBatch which sets up the restrictive policy automatically.
	agentCtx, err := app.NewAgentContext(ctx, app.ContextOptions{
		SrcDir:   r.SrcDir,
		LogLevel: 1,
		Mode:     app.ModeBatch, // <--- Crucial: Enforces non-interactive policy
		Policy:   policy.Batch,  // Explicitly set Batch policy
		Task:     job.Task,
	})

	if err != nil {
		r.failJob(job, "Failed to init context: "+err.Error())
		return
	}
	defer agentCtx.Close()

	// 2. Planning (Optional but recommended for Batch)
	// We use the planner to create structure, then execute it.
	// planner := agent.NewPlanner(agentCtx.Engine.Client, agentCtx.Engine.Model, r.Logger)

	// Create a simple one-shot plan if the task is simple, or a full plan for complex ones
	// For robustness in batch, we default to direct execution via Engine for now
	// unless we want to enforce planning for everything.
	// Let's stick to direct Engine execution for simplicity, OR use the planner if you prefer.
	// Below relies on the standard Engine run loop which is robust enough.

	printUpdate := func(msg openrouter.ChatMessage) {
		// Log thoughts to internal logger only, or update job progress in DB if you add that field
		if msg.Role == "assistant" && msg.Content != nil {
			// r.Logger.Debug(fmt.Sprintf("[Job %s] Thought: %v", job.ID[:8], msg.Content))
		}
	}

	// 3. Execution
	result, err := agentCtx.Engine.Run(ctx, job.Task, printUpdate)

	if err != nil {
		r.failJob(job, err.Error())
		return
	}

	// 4. Completion
	if err := r.Queue.UpdateStatus(job.ID, StatusCompleted, result, ""); err != nil {
		r.Logger.Error("Failed to mark complete: " + err.Error())
	}
	r.Logger.Info(fmt.Sprintf("✅ Job %s Completed", job.ID[:8]))
}

func (r *Runner) failJob(job *Job, reason string) {
	r.Logger.Error(fmt.Sprintf("❌ Job %s Failed: %s", job.ID[:8], reason))
	r.Queue.UpdateStatus(job.ID, StatusFailed, "", reason)
}
