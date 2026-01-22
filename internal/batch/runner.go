package batch

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/david22573/codepicker/internal/app"
	"github.com/david22573/codepicker/internal/database"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/policy"
	"github.com/david22573/codepicker/pkg/openrouter"
)

type Runner struct {
	Queue        *Queue
	Store        *database.Store
	Logger       logger.Logger
	Concurrency  int
	SrcDir       string
	shuttingDown bool
	mu           sync.Mutex
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

	// Phase 3: Graceful Shutdown Handling
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	// Create a cancellable context for workers
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	sem := make(chan struct{}, r.Concurrency)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	go func() {
		<-stopChan
		r.Logger.Warn("🛑 Shutdown signal received. Draining active jobs...")
		r.mu.Lock()
		r.shuttingDown = true
		r.mu.Unlock()
		cancel() // Signal workers to stop picking up NEW work
	}()

	for {
		// Check shutdown status
		r.mu.Lock()
		isDown := r.shuttingDown
		r.mu.Unlock()
		if isDown {
			break
		}

		select {
		case <-workerCtx.Done():
			goto DRAIN
		case <-ticker.C:
			select {
			case sem <- struct{}{}:
				// Acquired a worker slot
				job, err := r.Queue.Next()
				if err != nil {
					r.Logger.Error("Queue error: " + err.Error())
					<-sem
					continue
				}
				if job == nil {
					<-sem
					continue
				}

				wg.Add(1)
				go func(j *Job) {
					defer wg.Done()
					defer func() { <-sem }()
					// We use a fresh background context for the job itself so it finishes
					// even if the runner is shutting down (unless hard killed).
					r.processJob(context.Background(), j)
				}(job)
			default:
				continue
			}
		}
	}

DRAIN:
	r.Logger.Info("⏳ Waiting for active jobs to finish...")
	wg.Wait()
	r.Logger.Info("✅ Batch Runner stopped cleanly.")
	return nil
}

func (r *Runner) processJob(ctx context.Context, job *Job) {
	r.Logger.Info(fmt.Sprintf("👷 Picking up Job %s: %s", job.ID[:8], job.Task))

	if err := r.Queue.UpdateStatus(job.ID, StatusRunning, "", ""); err != nil {
		r.Logger.Error("Failed to update status: " + err.Error())
		return
	}

	agentCtx, err := app.NewAgentContext(ctx, app.ContextOptions{
		SrcDir:   r.SrcDir,
		LogLevel: 1,
		Mode:     app.ModeBatch,
		Policy:   policy.Batch,
		Task:     job.Task,
	})

	if err != nil {
		r.failJob(job, "Failed to init context: "+err.Error())
		return
	}
	defer agentCtx.Close()

	// Capture partial updates if needed, but for batch we mainly care about the final result
	printUpdate := func(msg openrouter.ChatMessage) {
		// Optional: Could log progress to a separate file or DB field
	}

	result, err := agentCtx.Engine.Run(ctx, job.Task, printUpdate)

	if err != nil {
		r.failJob(job, err.Error())
		return
	}

	// Phase 3: Pre-apply change summaries would be generated here by inspecting
	// the shadow manager. For now, we append the result.
	if err := r.Queue.UpdateStatus(job.ID, StatusCompleted, result, ""); err != nil {
		r.Logger.Error("Failed to mark complete: " + err.Error())
	}
	r.Logger.Info(fmt.Sprintf("✅ Job %s Completed", job.ID[:8]))
}

func (r *Runner) failJob(job *Job, reason string) {
	r.Logger.Error(fmt.Sprintf("❌ Job %s Failed: %s", job.ID[:8], reason))
	r.Queue.UpdateStatus(job.ID, StatusFailed, "", reason)
}
