package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/david22573/codepicker/internal/database"
	"github.com/david22573/codepicker/internal/shadow"
	"github.com/google/uuid"
)

// CheckpointManager handles creating and restoring checkpoints for agent sessions
type CheckpointManager struct {
	Store     *database.Store
	SessionID string
	Engine    *Engine
}

// NewCheckpointManager creates a new checkpoint manager
func NewCheckpointManager(store *database.Store, sessionID string, engine *Engine) *CheckpointManager {
	return &CheckpointManager{
		Store:     store,
		SessionID: sessionID,
		Engine:    engine,
	}
}

// CreateCheckpoint creates a checkpoint of the current agent state
func (cm *CheckpointManager) CreateCheckpoint(ctx context.Context, plan *Plan, currentStepIdx int) (*database.Checkpoint, error) {
	checkpoint := &database.Checkpoint{
		ID:          uuid.New().String(),
		SessionID:   cm.SessionID,
		Task:        "",
		Timestamp:   time.Now(),
		StepsStatus: make(map[int]string),
		StepResults: make(map[int]string),
		Metadata:    make(map[string]string),
	}

	// Add plan info if available
	if plan != nil {
		checkpoint.PlanID = plan.ID
		checkpoint.Task = plan.OriginalTask
		checkpoint.CurrentStep = currentStepIdx

		// Capture step statuses and results
		for _, step := range plan.Steps {
			checkpoint.StepsStatus[step.ID] = step.Status
			if step.Result != "" {
				checkpoint.StepResults[step.ID] = step.Result
			}
		}

		// Calculate progress
		completed := 0
		for _, status := range checkpoint.StepsStatus {
			if status == "completed" {
				completed++
			}
		}
		if len(plan.Steps) > 0 {
			checkpoint.Progress = float64(completed) / float64(len(plan.Steps))
		}

		// Determine status
		if checkpoint.Progress >= 1.0 {
			checkpoint.Status = database.CheckpointCompleted
		} else if currentStepIdx > 0 {
			checkpoint.Status = database.CheckpointActive
		} else {
			checkpoint.Status = database.CheckpointActive
		}
	}

	// Capture cost tracking
	if cm.Engine.CostTracker != nil {
		cost, count := cm.Engine.CostTracker.GetStats()
		checkpoint.TotalCost = cost
		checkpoint.RequestCount = count
	}

	// Capture session approvals
	if cm.Engine.Enforcer != nil {
		checkpoint.ApprovedWrite = cm.Engine.Enforcer.Session.AllowWrite
		checkpoint.ApprovedExec = cm.Engine.Enforcer.Session.AllowExec
	}

	// Capture memory snapshot
	if cm.Engine.Memory != nil && cm.Engine.Memory.Store != nil {
		snapshot, err := cm.Engine.Memory.Store.CreateSnapshot()
		if err == nil {
			checkpoint.MemorySnapshot = snapshot
		}
	}

	// Capture shadow workspace state
	shadowMgr := cm.getShadowManager()
	if shadowMgr != nil {
		shadowFiles := make(map[string]string)

		// Reload manifest to get latest state
		shadowMgr.LoadManifest()

		// Get all shadow files with their content hashes
		for path, meta := range shadowMgr.GetManifestChanges() {
			// Create a hash of the file metadata
			hash := sha256.Sum256([]byte(fmt.Sprintf("%s-%s-%v", path, meta.Agent, meta.Timestamp)))
			shadowFiles[path] = hex.EncodeToString(hash[:])
		}
		checkpoint.ShadowFiles = shadowFiles

		// Serialize the manifest
		manifestJSON, err := json.Marshal(shadowMgr.Manifest)
		if err == nil {
			checkpoint.ShadowManifest = string(manifestJSON)
		}
	}

	// Capture model information
	checkpoint.AgentModel = cm.Engine.Model
	checkpoint.WorkerModel = cm.Engine.WorkerModel
	if cm.Engine.Enforcer != nil && cm.Engine.Enforcer.Policy.Name != "" {
		checkpoint.PolicyName = cm.Engine.Enforcer.Policy.Name
	}

	// Save checkpoint to database
	if err := cm.Store.SaveCheckpoint(checkpoint); err != nil {
		return nil, fmt.Errorf("failed to save checkpoint: %w", err)
	}

	return checkpoint, nil
}

// RestoreCheckpoint restores the agent state from a checkpoint
func (cm *CheckpointManager) RestoreCheckpoint(ctx context.Context, checkpointID string) (*Plan, int, error) {
	checkpoint, err := cm.Store.LoadCheckpoint(checkpointID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to load checkpoint: %w", err)
	}

	// Restore session ID
	cm.SessionID = checkpoint.SessionID

	// Restore cost tracking
	if cm.Engine.CostTracker != nil {
		// Note: CostTracker doesn't have a restore method, so we'd need to add one
		// For now, we log the previous costs
		cm.Engine.Logger.Info(fmt.Sprintf("Previous session cost: $%.4f (%d requests)",
			checkpoint.TotalCost, checkpoint.RequestCount))
	}

	// Restore session approvals
	if cm.Engine.Enforcer != nil {
		cm.Engine.Enforcer.Session.AllowWrite = checkpoint.ApprovedWrite
		cm.Engine.Enforcer.Session.AllowExec = checkpoint.ApprovedExec

		if checkpoint.ApprovedWrite {
			cm.Engine.Logger.Info("🔓 Restored write approval from checkpoint")
		}
		if checkpoint.ApprovedExec {
			cm.Engine.Logger.Info("🔓 Restored exec approval from checkpoint")
		}
	}

	// Restore memory snapshot
	if checkpoint.MemorySnapshot != nil && cm.Engine.Memory != nil {
		if err := cm.Engine.Memory.Restore(checkpoint.MemorySnapshot); err != nil {
			cm.Engine.Logger.Warn(fmt.Sprintf("Failed to restore memory snapshot: %v", err))
		} else {
			cm.Engine.Logger.Info(fmt.Sprintf("✅ Restored %d files to working memory",
				len(checkpoint.MemorySnapshot.Files)))
		}
	}

	// Restore shadow workspace state
	shadowMgr := cm.getShadowManager()
	if shadowMgr != nil && checkpoint.ShadowManifest != "" {
		var manifest shadow.Manifest
		if err := json.Unmarshal([]byte(checkpoint.ShadowManifest), &manifest); err == nil {
			shadowMgr.Manifest = &manifest
			shadowMgr.SaveManifest()
			cm.Engine.Logger.Info(fmt.Sprintf("✅ Restored %d shadow files from checkpoint",
				len(manifest.Changes)))
		} else {
			cm.Engine.Logger.Warn(fmt.Sprintf("Failed to restore shadow manifest: %v", err))
		}
	}

	// Reconstruct the plan if we have a plan ID
	var plan *Plan
	if checkpoint.PlanID != "" {
		planRecord, err := cm.Store.GetPlan(checkpoint.PlanID)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to load plan %s: %w", checkpoint.PlanID, err)
		}

		var steps []Step
		if err := json.Unmarshal([]byte(planRecord.StepsJSON), &steps); err != nil {
			return nil, 0, fmt.Errorf("failed to parse plan steps: %w", err)
		}

		// Restore step statuses and results from checkpoint
		for i := range steps {
			stepID := steps[i].ID
			if status, ok := checkpoint.StepsStatus[stepID]; ok {
				steps[i].Status = status
			}
			if result, ok := checkpoint.StepResults[stepID]; ok {
				steps[i].Result = result
			}
		}

		plan = &Plan{
			ID:            checkpoint.PlanID,
			OriginalTask:  checkpoint.Task,
			Steps:         steps,
			EstimatedCost: planRecord.EstimatedCost,
		}
	}

	cm.Engine.Logger.Info(fmt.Sprintf("✅ Checkpoint restored: %s (Progress: %.1f%%, Step: %d)",
		checkpointID, checkpoint.Progress*100, checkpoint.CurrentStep))

	return plan, checkpoint.CurrentStep, nil
}

// AutoCheckpoint creates a checkpoint if auto-checkpoint is enabled
func (cm *CheckpointManager) AutoCheckpoint(ctx context.Context, plan *Plan, currentStepIdx int, reason string) error {
	checkpoint, err := cm.CreateCheckpoint(ctx, plan, currentStepIdx)
	if err != nil {
		cm.Engine.Logger.Warn(fmt.Sprintf("Auto-checkpoint failed: %v", err))
		return err
	}

	cm.Engine.Logger.Debug(fmt.Sprintf("📸 Auto-checkpoint created: %s (%s)", checkpoint.ID, reason))
	return nil
}

// ListCheckpoints returns all checkpoints for the current session
func (cm *CheckpointManager) ListCheckpoints() ([]database.CheckpointMetadata, error) {
	return cm.Store.ListCheckpoints(cm.SessionID)
}

// GetLatestCheckpoint retrieves the most recent checkpoint
func (cm *CheckpointManager) GetLatestCheckpoint() (*database.Checkpoint, error) {
	return cm.Store.GetLatestCheckpoint(cm.SessionID)
}

// CleanupOldCheckpoints removes checkpoints older than the specified duration
func (cm *CheckpointManager) CleanupOldCheckpoints(maxAge time.Duration) error {
	checkpoints, err := cm.ListCheckpoints()
	if err != nil {
		return err
	}

	cutoff := time.Now().Add(-maxAge)
	deleted := 0

	for _, cp := range checkpoints {
		if cp.Timestamp.Before(cutoff) && cp.Status != database.CheckpointActive {
			if err := cm.Store.DeleteCheckpoint(cp.ID); err != nil {
				cm.Engine.Logger.Warn(fmt.Sprintf("Failed to delete old checkpoint %s: %v", cp.ID, err))
			} else {
				deleted++
			}
		}
	}

	if deleted > 0 {
		cm.Engine.Logger.Info(fmt.Sprintf("🧹 Cleaned up %d old checkpoints", deleted))
	}

	return nil
}

// getShadowManager retrieves the shadow manager from the filesystem
func (cm *CheckpointManager) getShadowManager() *shadow.Manager {
	if cm.Engine.Memory == nil || cm.Engine.Memory.FS == nil {
		return nil
	}

	if overlay, ok := cm.Engine.Memory.FS.(interface{ GetShadowManager() *shadow.Manager }); ok {
		return overlay.GetShadowManager()
	}

	return nil
}
