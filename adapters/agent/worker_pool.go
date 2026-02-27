package agent

// ToolTask defines a unit of work for a tool execution.
type ToolTask func()

// ToolWorkerPool defines the boundary for executing tools concurrently.
// This prevents runaway goroutines when processing complex parallel plans.
type ToolWorkerPool interface {
	Submit(task ToolTask)
}

// BoundedWorkerPool implements ToolWorkerPool using a semaphore to limit concurrency.
type BoundedWorkerPool struct {
	sem chan struct{}
}

// NewBoundedWorkerPool creates a new pool allowing up to maxWorkers concurrent tasks.
func NewBoundedWorkerPool(maxWorkers int) *BoundedWorkerPool {
	return &BoundedWorkerPool{
		sem: make(chan struct{}, maxWorkers),
	}
}

// Submit queues a task and blocks if the pool is at maximum capacity.
func (p *BoundedWorkerPool) Submit(task ToolTask) {
	p.sem <- struct{}{} // Acquire slot
	go func() {
		defer func() { <-p.sem }() // Release slot
		task()
	}()
}