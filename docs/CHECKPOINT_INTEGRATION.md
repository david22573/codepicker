# Checkpoint System Integration Guide

## Overview

This guide explains how to integrate the checkpoint system into your codepicker workflows and applications.

## Quick Start

### Basic Usage

1. **Run agent task with automatic checkpointing**:
```bash
codepicker agent run "Your task here"
# Checkpoints are created automatically
```

2. **List available checkpoints**:
```bash
# List all sessions
codepicker checkpoint list

# List checkpoints for specific session
codepicker checkpoint list <session-id>
```

3. **Resume from checkpoint**:
```bash
codepicker agent resume <checkpoint-id>
```

### CLI Commands

| Command | Description |
|---------|-------------|
| `codepicker checkpoint list [session-id]` | List checkpoints |
| `codepicker checkpoint restore <checkpoint-id>` | Show checkpoint details |
| `codepicker agent resume <checkpoint-id>` | Resume execution |
| `codepicker checkpoint cleanup [--max-age 168h]` | Clean old checkpoints |
| `codepicker checkpoint delete <id> [--session]` | Delete checkpoint(s) |

## Programmatic Integration

### Basic Checkpoint Creation

```go
package main

import (
    "context"
    "github.com/david22573/codepicker/internal/agent"
    "github.com/david22573/codepicker/internal/database"
)

func main() {
    // Setup (engine, store, plan...)
    
    // Create checkpoint manager
    sessionID := uuid.New().String()
    cm := agent.NewCheckpointManager(store, sessionID, engine)
    
    // Create checkpoint
    checkpoint, err := cm.CreateCheckpoint(ctx, plan, currentStep)
    if err != nil {
        log.Fatal(err)
    }
    
    log.Printf("Checkpoint created: %s", checkpoint.ID)
}
```

### Plan Execution with Checkpointing

```go
// Create plan executor (checkpointing enabled by default)
executor := agent.NewPlanExecutor(engine, plan)

// Configure checkpointing behavior
executor.AutoCheckpoint = true      // Enable auto-checkpointing
executor.CheckpointInterval = 1     // Checkpoint every step

// Execute plan (checkpoints created automatically)
if err := executor.Execute(ctx); err != nil {
    log.Fatal(err)
}

// Access session ID for later resume
sessionID := executor.CheckpointManager.SessionID
log.Printf("Session: %s", sessionID)
```

### Resume from Checkpoint

```go
// Load checkpoint
checkpoint, err := store.LoadCheckpoint(checkpointID)
if err != nil {
    log.Fatal(err)
}

// Create executor with restored session
executor := agent.NewPlanExecutorWithSession(engine, nil, checkpoint.SessionID)

// Resume execution
if err := executor.Resume(ctx, checkpointID); err != nil {
    log.Fatal(err)
}
```

### Query Checkpoints

```go
// List all sessions
sessions, err := store.GetAllSessions()
if err != nil {
    log.Fatal(err)
}

for _, sessionID := range sessions {
    // Get checkpoints for session
    checkpoints, err := store.ListCheckpoints(sessionID)
    if err != nil {
        continue
    }
    
    log.Printf("Session %s: %d checkpoints", sessionID, len(checkpoints))
    
    for _, cp := range checkpoints {
        log.Printf("  - %s: %.1f%% complete", cp.ID, cp.Progress*100)
    }
}
```

### Get Latest Checkpoint

```go
// Get the most recent checkpoint for a session
latest, err := cm.GetLatestCheckpoint()
if err != nil {
    log.Fatal(err)
}

log.Printf("Latest: %s (%.1f%%)", latest.ID, latest.Progress*100)
```

### Manual Checkpoint Creation

```go
// Create checkpoint at specific points
checkpoint, err := cm.CreateCheckpoint(ctx, plan, currentStep)
if err != nil {
    log.Warn("Failed to checkpoint: %v", err)
}

// Update status later
store.UpdateCheckpointStatus(checkpoint.ID, database.CheckpointCompleted)
```

## Advanced Integration

### Custom Checkpoint Intervals

```go
// Checkpoint every 5 steps
executor.CheckpointInterval = 5

// Disable auto-checkpointing, use manual only
executor.AutoCheckpoint = false

// Manual checkpoint at critical points
if criticalOperation {
    cm.AutoCheckpoint(ctx, plan, currentStep, "before_critical_op")
}
```

### Checkpoint Metadata

```go
checkpoint := &database.Checkpoint{
    // ... standard fields ...
    
    Metadata: map[string]string{
        "branch": "feature/new-api",
        "commit": "abc123",
        "environment": "production",
        "user": "alice",
    },
}

store.SaveCheckpoint(checkpoint)
```

### Conditional Checkpointing

```go
// Checkpoint only on expensive operations
if estimatedCost > 0.10 {
    cm.AutoCheckpoint(ctx, plan, currentStep, "expensive_operation")
}

// Checkpoint before risky operations
if operation.IsRisky() {
    checkpoint, _ := cm.CreateCheckpoint(ctx, plan, currentStep)
    log.Printf("Safety checkpoint: %s", checkpoint.ID)
}
```

### Error Recovery with Checkpoints

```go
// Execute with checkpoint-based recovery
for attempt := 0; attempt < maxAttempts; attempt++ {
    // Create safety checkpoint
    safetyCP, _ := cm.CreateCheckpoint(ctx, plan, currentStep)
    
    err := executeRiskyStep(plan.Steps[currentStep])
    if err == nil {
        break
    }
    
    // Restore from safety checkpoint
    log.Printf("Error occurred, restoring checkpoint %s", safetyCP.ID)
    plan, currentStep, _ = cm.RestoreCheckpoint(ctx, safetyCP.ID)
    
    // Try again with different approach
    modifyApproach(plan.Steps[currentStep])
}
```

### Checkpoint Cleanup Policies

```go
// Cleanup based on status
checkpoints, _ := store.ListCheckpoints(sessionID)
for _, cp := range checkpoints {
    if cp.Status == database.CheckpointCompleted {
        // Keep only last 3 completed checkpoints
        if len(completedCheckpoints) > 3 {
            store.DeleteCheckpoint(cp.ID)
        }
    }
}

// Cleanup by age
cm.CleanupOldCheckpoints(7 * 24 * time.Hour)

// Cleanup by progress
for _, cp := range checkpoints {
    if cp.Progress < 0.1 {
        // Delete checkpoints with minimal progress
        store.DeleteCheckpoint(cp.ID)
    }
}
```

## Integration Patterns

### Pattern 1: Long-Running Job Checkpointing

```go
type CheckpointedJob struct {
    job         *Job
    executor    *agent.PlanExecutor
    checkpoints []string
}

func (cj *CheckpointedJob) Run(ctx context.Context) error {
    // Setup
    cj.executor.AutoCheckpoint = true
    cj.executor.CheckpointInterval = 1
    
    // Execute with checkpointing
    err := cj.executor.Execute(ctx)
    if err != nil {
        // Save checkpoint ID for resume
        latest, _ := cj.executor.CheckpointManager.GetLatestCheckpoint()
        cj.job.ResumeCheckpoint = latest.ID
        return err
    }
    
    return nil
}

func (cj *CheckpointedJob) Resume(ctx context.Context) error {
    return cj.executor.Resume(ctx, cj.job.ResumeCheckpoint)
}
```

### Pattern 2: Cost-Aware Checkpointing

```go
type CostAwareExecutor struct {
    executor   *agent.PlanExecutor
    maxCost    float64
    checkpoint string
}

func (ce *CostAwareExecutor) Execute(ctx context.Context) error {
    // Monitor cost during execution
    go ce.monitorCost(ctx)
    
    err := ce.executor.Execute(ctx)
    return err
}

func (ce *CostAwareExecutor) monitorCost(ctx context.Context) {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            cost, _ := ce.executor.Engine.CostTracker.GetStats()
            
            // Create checkpoint before hitting limit
            if cost > ce.maxCost * 0.9 {
                cp, _ := ce.executor.CheckpointManager.CreateCheckpoint(
                    ctx, 
                    ce.executor.Plan, 
                    getCurrentStep(),
                )
                ce.checkpoint = cp.ID
                
                log.Printf("Cost limit approaching, checkpoint: %s", cp.ID)
                return
            }
        }
    }
}
```

### Pattern 3: Multi-Stage Pipeline with Checkpoints

```go
type Pipeline struct {
    stages   []Stage
    executor *agent.PlanExecutor
}

func (p *Pipeline) Run(ctx context.Context) error {
    for i, stage := range p.stages {
        // Checkpoint before each stage
        cp, err := p.executor.CheckpointManager.CreateCheckpoint(
            ctx, 
            p.executor.Plan, 
            i,
        )
        if err != nil {
            log.Warn("Failed to checkpoint: %v", err)
        }
        
        log.Printf("Stage %d checkpoint: %s", i, cp.ID)
        
        // Execute stage
        if err := stage.Execute(ctx); err != nil {
            // Stage failed, can resume from checkpoint later
            return fmt.Errorf("stage %d failed (checkpoint: %s): %w", i, cp.ID, err)
        }
    }
    
    return nil
}
```

## Database Management

### Backup Checkpoints

```bash
# Backup checkpoint database
cp .codepicker/codepicker.db .codepicker/codepicker.db.backup

# Or use SQLite backup
sqlite3 .codepicker/codepicker.db ".backup .codepicker/backup.db"
```

### Restore Checkpoints

```bash
# Restore from backup
cp .codepicker/backup.db .codepicker/codepicker.db
```

### Export Checkpoints

```go
// Export checkpoint to JSON
checkpoint, _ := store.LoadCheckpoint(checkpointID)
data, _ := json.MarshalIndent(checkpoint, "", "  ")
os.WriteFile("checkpoint.json", data, 0644)

// Import checkpoint from JSON
data, _ := os.ReadFile("checkpoint.json")
var checkpoint database.Checkpoint
json.Unmarshal(data, &checkpoint)
store.SaveCheckpoint(&checkpoint)
```

### Migrate Checkpoints

```go
// Migrate old checkpoints to new schema
oldCheckpoints, _ := oldStore.ListCheckpoints(sessionID)
for _, oldCP := range oldCheckpoints {
    newCP := ConvertToNewFormat(oldCP)
    newStore.SaveCheckpoint(newCP)
}
```

## Testing with Checkpoints

### Unit Tests

```go
func TestCheckpointCreation(t *testing.T) {
    store, _ := database.New(t.TempDir())
    defer store.Close()
    
    cm := agent.NewCheckpointManager(store, "test-session", engine)
    
    checkpoint, err := cm.CreateCheckpoint(ctx, plan, 0)
    assert.NoError(t, err)
    assert.NotEmpty(t, checkpoint.ID)
}

func TestCheckpointRestore(t *testing.T) {
    // Create checkpoint
    checkpoint, _ := cm.CreateCheckpoint(ctx, plan, 0)
    
    // Restore in new manager
    newCM := agent.NewCheckpointManager(store, "new-session", engine)
    restoredPlan, step, err := newCM.RestoreCheckpoint(ctx, checkpoint.ID)
    
    assert.NoError(t, err)
    assert.Equal(t, plan.ID, restoredPlan.ID)
}
```

### Integration Tests

```go
func TestEndToEndCheckpointing(t *testing.T) {
    // Execute plan with checkpointing
    executor := agent.NewPlanExecutor(engine, plan)
    executor.AutoCheckpoint = true
    
    // Simulate interruption
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    err := executor.Execute(ctx)
    // Might timeout, but checkpoint should exist
    
    // Get latest checkpoint
    latest, err := executor.CheckpointManager.GetLatestCheckpoint()
    assert.NoError(t, err)
    
    // Resume from checkpoint
    newExecutor := agent.NewPlanExecutorWithSession(engine, nil, latest.SessionID)
    err = newExecutor.Resume(context.Background(), latest.ID)
    assert.NoError(t, err)
}
```

## Monitoring and Observability

### Checkpoint Metrics

```go
// Collect checkpoint metrics
type CheckpointMetrics struct {
    TotalCheckpoints    int
    ActiveCheckpoints   int
    CompletedSessions   int
    AverageCheckpointSize int64
    OldestCheckpoint    time.Time
}

func collectMetrics(store *database.Store) CheckpointMetrics {
    sessions, _ := store.GetAllSessions()
    
    var metrics CheckpointMetrics
    metrics.TotalCheckpoints = 0
    
    for _, sessionID := range sessions {
        cps, _ := store.ListCheckpoints(sessionID)
        metrics.TotalCheckpoints += len(cps)
        
        for _, cp := range cps {
            if cp.Status == database.CheckpointActive {
                metrics.ActiveCheckpoints++
            }
        }
    }
    
    return metrics
}
```

### Logging Integration

```go
// Log checkpoint events
logger.Info("Checkpoint created", 
    "id", checkpoint.ID,
    "session", checkpoint.SessionID,
    "progress", checkpoint.Progress,
    "cost", checkpoint.TotalCost,
)

// Structured logging
log.WithFields(log.Fields{
    "checkpoint_id": checkpoint.ID,
    "session_id":    checkpoint.SessionID,
    "step":          checkpoint.CurrentStep,
    "status":        checkpoint.Status,
}).Info("Checkpoint created")
```

## Best Practices Summary

1. **Enable auto-checkpointing for all long-running tasks**
2. **Checkpoint before expensive or risky operations**
3. **Clean up old checkpoints regularly** (7 days recommended)
4. **Use meaningful metadata** for tracking and debugging
5. **Test checkpoint restore in your CI pipeline**
6. **Monitor checkpoint database size**
7. **Backup checkpoint database periodically**
8. **Use checkpoints for cost management**
9. **Document checkpoint IDs in logs**
10. **Set up alerts for failed checkpoint restoration**

## Troubleshooting

See [CHECKPOINTS.md](CHECKPOINTS.md#troubleshooting) for detailed troubleshooting guide.

## See Also

- [Checkpoint System Documentation](CHECKPOINTS.md)
- [Database Schema](../internal/database/schema.go)
- [Checkpoint Manager API](../internal/agent/checkpoint.go)
- [Example: Checkpoint Demo](../examples/checkpoint_demo.go)
