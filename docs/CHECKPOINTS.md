# Checkpoint System for Resumable Agent Sessions

## Overview

The checkpoint system allows agent sessions to be saved and resumed at any point during execution. This is crucial for:

- **Long-running tasks**: Resume execution after interruptions or failures
- **Cost management**: Pause execution and resume later without losing progress
- **Experimentation**: Try different approaches from the same starting point
- **Debugging**: Inspect and restore agent state at specific execution points

## Features

### Comprehensive State Capture

Each checkpoint captures:

- **Execution State**: Current step, step statuses, step results, turn count, error history
- **Cost Tracking**: Total cost and request count at checkpoint time
- **Session Approvals**: Write and exec permissions (for interactive mode)
- **Working Memory**: All files currently in agent memory
- **Shadow Workspace**: All pending changes in the shadow filesystem
- **Agent Configuration**: Models, policy, and metadata

### Automatic Checkpointing

By default, checkpoints are created automatically:

- At execution start (before any work begins)
- After each completed step (configurable interval)
- On step failure (before returning error)
- At execution completion

### Manual Control

You can also create and manage checkpoints manually using CLI commands.

## Usage

### Automatic Checkpointing (Default)

Checkpoints are created automatically during plan execution:

```bash
# Run agent task - checkpoints created automatically
codepicker agent run "Implement feature X"
```

### Resume from Checkpoint

List available sessions and their checkpoints:

```bash
# List all sessions
codepicker checkpoint list

# List checkpoints for a specific session
codepicker checkpoint list <session-id>
```

Resume execution from a specific checkpoint:

```bash
# Resume from checkpoint
codepicker agent resume <checkpoint-id>

# Resume in CI mode (no interactive prompts)
codepicker agent resume <checkpoint-id> --ci
```

### Checkpoint Management

View checkpoint details:

```bash
# Show checkpoint information
codepicker checkpoint restore <checkpoint-id>
```

Clean up old checkpoints:

```bash
# Remove checkpoints older than 7 days (default)
codepicker checkpoint cleanup

# Custom age threshold
codepicker checkpoint cleanup --max-age 48h

# Clean specific session
codepicker checkpoint cleanup <session-id>
```

Delete checkpoints:

```bash
# Delete a single checkpoint
codepicker checkpoint delete <checkpoint-id>

# Delete all checkpoints for a session
codepicker checkpoint delete <session-id> --session
```

## Checkpoint Data Structure

### Core Fields

```go
type Checkpoint struct {
    ID        string    // Unique checkpoint identifier
    SessionID string    // Session this checkpoint belongs to
    PlanID    string    // Associated plan ID (if any)
    Task      string    // Original task description
    Timestamp time.Time // When checkpoint was created
    
    // Execution State
    CurrentStep  int               // Current step being executed
    StepsStatus  map[int]string    // Status of each step (pending/running/completed/failed)
    StepResults  map[int]string    // Results from completed steps
    TurnCount    int               // Total agent turns executed
    ErrorCount   int               // Number of errors encountered
    LastError    string            // Most recent error (if any)
    LastToolUsed string            // Last tool executed
    Progress     float64           // Completion percentage (0.0 to 1.0)
    Status       CheckpointStatus  // Checkpoint status
    
    // Cost Tracking
    TotalCost    float64 // Cumulative cost at checkpoint time
    RequestCount int     // Number of API requests made
    
    // Session Approvals
    ApprovedWrite bool // Whether write access was granted
    ApprovedExec  bool // Whether exec access was granted
    
    // Memory State
    MemorySnapshot *MemorySnapshot // Complete working memory snapshot
    
    // Shadow Workspace State
    ShadowFiles    map[string]string // Shadow files and their hashes
    ShadowManifest string            // Serialized shadow manifest
    
    // Metadata
    AgentModel  string            // Model used for supervision
    WorkerModel string            // Model used for work execution
    PolicyName  string            // Security policy in effect
    Metadata    map[string]string // Additional metadata
}
```

### Checkpoint Status

- `active`: Checkpoint represents ongoing work
- `paused`: Execution paused at this checkpoint
- `completed`: Execution completed successfully
- `failed`: Execution failed at or after this checkpoint
- `cancelled`: Execution was cancelled by user

## Implementation Details

### Checkpoint Creation

Checkpoints are created by the `CheckpointManager`:

```go
// Create checkpoint at current state
checkpoint, err := checkpointManager.CreateCheckpoint(ctx, plan, currentStepIdx)
```

The manager:
1. Captures current plan state (steps, statuses, results)
2. Calculates progress percentage
3. Records cost and request metrics
4. Takes a snapshot of working memory
5. Records shadow filesystem state
6. Saves session approvals
7. Persists everything to SQLite database

### State Restoration

Restoring from a checkpoint:

```go
// Restore state from checkpoint
plan, currentStep, err := checkpointManager.RestoreCheckpoint(ctx, checkpointID)
```

The restore process:
1. Loads checkpoint from database
2. Restores session approvals to enforcer
3. Restores working memory from snapshot
4. Restores shadow filesystem manifest
5. Reconstructs plan with saved step statuses
6. Returns plan and current step index

### Integration with PlanExecutor

The `PlanExecutor` integrates checkpointing seamlessly:

```go
type PlanExecutor struct {
    Engine             *Engine
    Plan               *Plan
    CheckpointManager  *CheckpointManager
    AutoCheckpoint     bool // Enable automatic checkpointing
    CheckpointInterval int  // Checkpoint every N steps
}

// Resume execution from checkpoint
func (pe *PlanExecutor) Resume(ctx context.Context, checkpointID string) error {
    // Restore state
    restoredPlan, currentStep, err := pe.CheckpointManager.RestoreCheckpoint(ctx, checkpointID)
    
    // Update executor state
    pe.Plan = restoredPlan
    
    // Continue execution from restored step
    return pe.Execute(ctx)
}
```

### Database Schema

Checkpoints are stored in SQLite with the following schema:

```sql
CREATE TABLE checkpoints (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    plan_id TEXT,
    task TEXT NOT NULL,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    
    -- Execution State
    current_step INTEGER DEFAULT 0,
    steps_status TEXT,      -- JSON
    step_results TEXT,      -- JSON
    turn_count INTEGER DEFAULT 0,
    error_count INTEGER DEFAULT 0,
    last_error TEXT,
    last_tool_used TEXT,
    progress REAL DEFAULT 0.0,
    status TEXT DEFAULT 'active',
    
    -- Cost Tracking
    total_cost REAL DEFAULT 0.0,
    request_count INTEGER DEFAULT 0,
    
    -- Session Approvals
    approved_write BOOLEAN DEFAULT 0,
    approved_exec BOOLEAN DEFAULT 0,
    
    -- Memory State
    memory_snapshot TEXT,   -- JSON
    
    -- Shadow Workspace State
    shadow_files TEXT,      -- JSON
    shadow_manifest TEXT,   -- JSON
    
    -- Metadata
    agent_model TEXT,
    worker_model TEXT,
    policy_name TEXT,
    metadata TEXT           -- JSON
);

CREATE INDEX idx_checkpoints_session ON checkpoints(session_id);
CREATE INDEX idx_checkpoints_timestamp ON checkpoints(timestamp DESC);
CREATE INDEX idx_checkpoints_status ON checkpoints(status);
```

## Configuration

### Customize Checkpoint Behavior

```go
// Create executor with custom checkpoint settings
executor := agent.NewPlanExecutor(engine, plan)

// Disable auto-checkpointing
executor.AutoCheckpoint = false

// Checkpoint every 3 steps instead of every step
executor.CheckpointInterval = 3

// Execute
executor.Execute(ctx)
```

### Checkpoint Cleanup

Configure automatic cleanup in your workflow:

```go
// Clean up checkpoints older than 24 hours
err := checkpointManager.CleanupOldCheckpoints(24 * time.Hour)
```

Or use the CLI:

```bash
# Add to cron or CI cleanup job
codepicker checkpoint cleanup --max-age 24h
```

## Best Practices

### 1. Regular Checkpointing for Long Tasks

For tasks that take more than a few minutes, keep the default checkpoint interval of 1 (checkpoint after every step).

### 2. Clean Up Old Checkpoints

Checkpoints can consume disk space. Clean up periodically:

```bash
# Weekly cleanup in cron
0 0 * * 0 codepicker checkpoint cleanup --max-age 168h
```

### 3. Resume from Latest Checkpoint

If a task fails, you can quickly resume:

```bash
# Get the latest checkpoint for a session
SESSION_ID=$(codepicker checkpoint list | grep "Session:" | head -1 | awk '{print $2}')
LATEST_CP=$(codepicker checkpoint list $SESSION_ID | awk 'NR==3 {print $1}')

# Resume
codepicker agent resume $LATEST_CP
```

### 4. Use Checkpoints for Experimentation

Create a checkpoint before trying risky operations:

```go
// Create manual checkpoint before risky operation
checkpoint, _ := checkpointManager.CreateCheckpoint(ctx, plan, currentStep)
fmt.Printf("Checkpoint created: %s\n", checkpoint.ID)

// If things go wrong, restore from this checkpoint
```

### 5. Cost Management

Monitor costs using checkpoint data:

```bash
# View cost progression across checkpoints
codepicker checkpoint list <session-id> | awk '{print $6}'
```

## Troubleshooting

### Checkpoint Not Found

**Problem**: `checkpoint not found` error when trying to resume

**Solutions**:
- Verify checkpoint ID: `codepicker checkpoint list`
- Check if checkpoint was deleted
- Ensure `.codepicker/` directory hasn't been removed

### Memory Snapshot Fails

**Problem**: Checkpoint creation succeeds but memory isn't restored

**Solutions**:
- Check for database corruption: `sqlite3 .codepicker/codepicker.db "PRAGMA integrity_check;"`
- Ensure sufficient disk space
- Review logs for serialization errors

### Session Approvals Not Restored

**Problem**: After resuming, agent prompts for approvals again

**Solutions**:
- Verify checkpoint was created in interactive mode
- Check that `ApprovedWrite` and `ApprovedExec` flags are set in checkpoint
- Ensure you're not running in a different policy mode (e.g., `--ci` flag)

## Performance Considerations

### Checkpoint Size

Checkpoints include full memory snapshots and can grow large:

- **Small projects**: ~10-50 KB per checkpoint
- **Large projects**: ~500 KB - 2 MB per checkpoint
- **Very large contexts**: Up to 5-10 MB per checkpoint

Monitor checkpoint size:

```bash
du -sh .codepicker/codepicker.db
```

### Creation Overhead

Checkpoint creation adds:
- ~50-100ms for small contexts
- ~200-500ms for large contexts
- Minimal impact on overall execution time

### Database Size Management

SQLite database will grow over time. Compact periodically:

```bash
# Compact database
sqlite3 .codepicker/codepicker.db "VACUUM;"
```

## Future Enhancements

Potential improvements being considered:

- [ ] Checkpoint compression to reduce disk usage
- [ ] Cloud backup/restore for checkpoints
- [ ] Checkpoint diffing to see what changed
- [ ] Time-travel debugging using checkpoints
- [ ] Checkpoint branching for parallel experimentation
- [ ] Automatic checkpoint pruning based on policies

## See Also

- [Agent Architecture](ARCHITECTURE.md)
- [Cost Tracking](COST_TRACKING.md)
- [Shadow Filesystem](SHADOW_FS.md)
