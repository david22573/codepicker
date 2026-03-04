package agent

import "sync"

// ToolTask defines a unit of work for a tool execution.
type ToolTask func()

// ToolWorkerPool defines the boundary for executing tools concurrently.
// This prevents runaway goroutines when processing complex parallel plans.
type ToolWorkerPool interface {
	Submit(task ToolTask)
	Close()
}

// BoundedWorkerPool implements ToolWorkerPool using a fixed set of long-lived workers.
type BoundedWorkerPool struct {
	tasks chan ToolTask
	wg    sync.WaitGroup
}

// NewBoundedWorkerPool creates a new pool with maxWorkers concurrent background goroutines.
func NewBoundedWorkerPool(maxWorkers int) *BoundedWorkerPool {
	p := &BoundedWorkerPool{
		// Buffer allows producers to queue up work without immediately blocking
		tasks: make(chan ToolTask, maxWorkers*2),
	}

	p.wg.Add(maxWorkers)
	for i := 0; i < maxWorkers; i++ {
		go func() {
			defer p.wg.Done()
			// Workers stay alive and pull tasks until the channel is closed
			for task := range p.tasks {
				task()
			}
		}()
	}

	return p
}

// Submit queues a task. It will block if the queue is full, applying natural backpressure.
func (p *BoundedWorkerPool) Submit(task ToolTask) {
	p.tasks <- task
}

// Close signals all workers to shut down after finishing their current tasks.
func (p *BoundedWorkerPool) Close() {
	close(p.tasks)
	p.wg.Wait()
}
