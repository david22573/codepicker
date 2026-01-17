package batch

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/david22573/codepicker/internal/agent"
	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/constants"
	"github.com/david22573/codepicker/internal/database"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/pkg/openrouter"
)

type Runner struct {
	Queue       *Queue
	Store       *database.Store
	Client      *openrouter.Client
	Logger      logger.Logger
	Concurrency int
	SrcDir      string
	APIKey      string
}

func NewRunner(q *Queue, s *database.Store, client *openrouter.Client, log logger.Logger, workers int, srcDir, apiKey string) *Runner {
	if workers < 1 {
		workers = 1
	}
	return &Runner{
		Queue:       q,
		Store:       s,
		Client:      client,
		Logger:      log,
		Concurrency: workers,
		SrcDir:      srcDir,
		APIKey:      apiKey,
	}
}

func (r *Runner) Start(ctx context.Context) error {
	r.Logger.Info(fmt.Sprintf("🚀 Starting Batch Runner with %d workers", r.Concurrency))

	var wg sync.WaitGroup
	sem := make(chan struct{}, r.Concurrency)

	// Polling loop
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.Logger.Info("🛑 Batch Runner stopping...")
			wg.Wait()
			return nil
		case <-ticker.C:
			// Try to acquire a worker slot
			select {
			case sem <- struct{}{}:
				// Slot acquired, check for job
				job, err := r.Queue.Next()
				if err != nil {
					r.Logger.Error("Queue error: " + err.Error())
					<-sem // Release slot
					continue
				}
				if job == nil {
					<-sem // Release slot, no job
					continue
				}

				wg.Add(1)
				go func(j *Job) {
					defer wg.Done()
					defer func() { <-sem }() // Release slot when done
					r.processJob(ctx, j)
				}(job)

			default:
				// All workers busy
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

	// Initialize a fresh engine for this job
	// NOTE: We need a fresh engine per job to ensure clean memory/history if desired,
	// or we pass a shared DB. Here we use the shared DB but a new Engine instance.
	limits := config.DefaultLimits()

	// Create absolute path safely
	absSrc, _ := filepath.Abs(r.SrcDir)

	eng, err := agent.NewEngine(r.Client, constants.DefaultModel, absSrc, r.Logger, limits, r.Store)
	if err != nil {
		r.failJob(job, "Failed to init engine: "+err.Error())
		return
	}

	// ⚠️ AUTO-APPROVAL for Batch Jobs
	// We assume if you queued it, you want it run.
	eng.ApprovalCallback = func(cmd, reason string) bool {
		r.Logger.Info(fmt.Sprintf("[Job %s] Auto-approving: %s", job.ID[:8], cmd))
		return true
	}

	// Define a printer that logs thoughts but doesn't spam stdout too much
	printUpdate := func(msg openrouter.ChatMessage) {
		if msg.Role == "assistant" && msg.Content != nil {
			content := fmt.Sprintf("%v", msg.Content)
			if content != "" {
				// Log to debug/file in real life, print to stdout for now
				// r.Logger.Debug(fmt.Sprintf("[Job %s] Thought: %s", job.ID[:8], content))
			}
		}
	}

	// Execution
	// Option A: Just run the engine
	// result, err := eng.Run(ctx, job.Task, printUpdate)

	// Option B (Robust): Use Planner -> Executor (Phase 1 logic)
	// This is better because it breaks down complex batch tasks
	planner := agent.NewPlanner(r.Client, constants.DefaultModel, r.Logger)
	plan, err := planner.CreatePlan(ctx, job.Task, "See Context") // We rely on tool usage for context reading

	var result string
	if err == nil {
		executor := agent.NewPlanExecutor(eng, plan)
		if err := executor.Execute(ctx); err != nil {
			err = fmt.Errorf("plan execution failed: %w", err)
		} else {
			result = "Plan executed successfully. Check shadow directory."
		}
	} else {
		// Fallback to simple run if planning fails
		r.Logger.Warn(fmt.Sprintf("[Job %s] Planning failed, falling back to direct execution: %v", job.ID[:8], err))
		result, err = eng.Run(ctx, job.Task, printUpdate)
	}

	if err != nil {
		r.failJob(job, err.Error())
		return
	}

	if err := r.Queue.UpdateStatus(job.ID, StatusCompleted, result, ""); err != nil {
		r.Logger.Error("Failed to mark complete: " + err.Error())
	}
	r.Logger.Info(fmt.Sprintf("✅ Job %s Completed", job.ID[:8]))
}

func (r *Runner) failJob(job *Job, reason string) {
	r.Logger.Error(fmt.Sprintf("❌ Job %s Failed: %s", job.ID[:8], reason))
	r.Queue.UpdateStatus(job.ID, StatusFailed, "", reason)
}
