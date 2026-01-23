package database

import (
	"encoding/json"
	"fmt"
	"time"
)

// Checkpoint represents a complete snapshot of an agent session
type Checkpoint struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	PlanID    string    `json:"plan_id,omitempty"`
	Task      string    `json:"task"`
	Timestamp time.Time `json:"timestamp"`

	// Execution State
	CurrentStep  int              `json:"current_step"`
	StepsStatus  map[int]string   `json:"steps_status"` // step_id -> status
	StepResults  map[int]string   `json:"step_results"` // step_id -> result
	TurnCount    int              `json:"turn_count"`
	ErrorCount   int              `json:"error_count"`
	LastError    string           `json:"last_error,omitempty"`
	LastToolUsed string           `json:"last_tool_used,omitempty"`
	Progress     float64          `json:"progress"` // 0.0 to 1.0
	Status       CheckpointStatus `json:"status"`

	// Cost Tracking
	TotalCost    float64 `json:"total_cost"`
	RequestCount int     `json:"request_count"`

	// Session Approvals (for interactive mode)
	ApprovedWrite bool `json:"approved_write"`
	ApprovedExec  bool `json:"approved_exec"`

	// Memory State
	MemorySnapshot *MemorySnapshot `json:"memory_snapshot,omitempty"`

	// Shadow Workspace State
	ShadowFiles    map[string]string `json:"shadow_files,omitempty"`    // path -> content_hash
	ShadowManifest string            `json:"shadow_manifest,omitempty"` // Serialized manifest

	// Metadata
	AgentModel  string            `json:"agent_model"`
	WorkerModel string            `json:"worker_model"`
	PolicyName  string            `json:"policy_name"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// CheckpointStatus represents the state of a checkpoint
type CheckpointStatus string

const (
	CheckpointActive    CheckpointStatus = "active"
	CheckpointPaused    CheckpointStatus = "paused"
	CheckpointCompleted CheckpointStatus = "completed"
	CheckpointFailed    CheckpointStatus = "failed"
	CheckpointCancelled CheckpointStatus = "cancelled"
)

// CheckpointMetadata provides summary information about checkpoints
type CheckpointMetadata struct {
	ID          string           `json:"id"`
	SessionID   string           `json:"session_id"`
	Task        string           `json:"task"`
	Timestamp   time.Time        `json:"timestamp"`
	Status      CheckpointStatus `json:"status"`
	Progress    float64          `json:"progress"`
	TotalCost   float64          `json:"total_cost"`
	CurrentStep int              `json:"current_step"`
	TurnCount   int              `json:"turn_count"`
}

// SaveCheckpoint saves a complete checkpoint to the database
func (s *Store) SaveCheckpoint(cp *Checkpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Serialize complex fields
	stepsStatusJSON, err := json.Marshal(cp.StepsStatus)
	if err != nil {
		return fmt.Errorf("failed to serialize steps_status: %w", err)
	}

	stepResultsJSON, err := json.Marshal(cp.StepResults)
	if err != nil {
		return fmt.Errorf("failed to serialize step_results: %w", err)
	}

	memorySnapshotJSON, err := json.Marshal(cp.MemorySnapshot)
	if err != nil {
		return fmt.Errorf("failed to serialize memory_snapshot: %w", err)
	}

	shadowFilesJSON, err := json.Marshal(cp.ShadowFiles)
	if err != nil {
		return fmt.Errorf("failed to serialize shadow_files: %w", err)
	}

	metadataJSON, err := json.Marshal(cp.Metadata)
	if err != nil {
		return fmt.Errorf("failed to serialize metadata: %w", err)
	}

	_, err = s.db.Exec(`
		INSERT INTO checkpoints (
			id, session_id, plan_id, task, timestamp,
			current_step, steps_status, step_results, turn_count, error_count, last_error, last_tool_used, progress, status,
			total_cost, request_count,
			approved_write, approved_exec,
			memory_snapshot,
			shadow_files, shadow_manifest,
			agent_model, worker_model, policy_name, metadata
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			current_step=excluded.current_step,
			steps_status=excluded.steps_status,
			step_results=excluded.step_results,
			turn_count=excluded.turn_count,
			error_count=excluded.error_count,
			last_error=excluded.last_error,
			last_tool_used=excluded.last_tool_used,
			progress=excluded.progress,
			status=excluded.status,
			total_cost=excluded.total_cost,
			request_count=excluded.request_count,
			approved_write=excluded.approved_write,
			approved_exec=excluded.approved_exec,
			memory_snapshot=excluded.memory_snapshot,
			shadow_files=excluded.shadow_files,
			shadow_manifest=excluded.shadow_manifest,
			timestamp=excluded.timestamp
	`,
		cp.ID, cp.SessionID, cp.PlanID, cp.Task, cp.Timestamp,
		cp.CurrentStep, string(stepsStatusJSON), string(stepResultsJSON), cp.TurnCount, cp.ErrorCount, cp.LastError, cp.LastToolUsed, cp.Progress, string(cp.Status),
		cp.TotalCost, cp.RequestCount,
		cp.ApprovedWrite, cp.ApprovedExec,
		string(memorySnapshotJSON),
		string(shadowFilesJSON), cp.ShadowManifest,
		cp.AgentModel, cp.WorkerModel, cp.PolicyName, string(metadataJSON),
	)

	return err
}

// LoadCheckpoint retrieves a checkpoint from the database
func (s *Store) LoadCheckpoint(id string) (*Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var cp Checkpoint
	var stepsStatusJSON, stepResultsJSON, memorySnapshotJSON, shadowFilesJSON, metadataJSON string
	var status string

	err := s.db.QueryRow(`
		SELECT 
			id, session_id, plan_id, task, timestamp,
			current_step, steps_status, step_results, turn_count, error_count, last_error, last_tool_used, progress, status,
			total_cost, request_count,
			approved_write, approved_exec,
			memory_snapshot,
			shadow_files, shadow_manifest,
			agent_model, worker_model, policy_name, metadata
		FROM checkpoints
		WHERE id = ?
	`, id).Scan(
		&cp.ID, &cp.SessionID, &cp.PlanID, &cp.Task, &cp.Timestamp,
		&cp.CurrentStep, &stepsStatusJSON, &stepResultsJSON, &cp.TurnCount, &cp.ErrorCount, &cp.LastError, &cp.LastToolUsed, &cp.Progress, &status,
		&cp.TotalCost, &cp.RequestCount,
		&cp.ApprovedWrite, &cp.ApprovedExec,
		&memorySnapshotJSON,
		&shadowFilesJSON, &cp.ShadowManifest,
		&cp.AgentModel, &cp.WorkerModel, &cp.PolicyName, &metadataJSON,
	)

	if err != nil {
		return nil, fmt.Errorf("checkpoint not found: %w", err)
	}

	cp.Status = CheckpointStatus(status)

	// Deserialize JSON fields
	if err := json.Unmarshal([]byte(stepsStatusJSON), &cp.StepsStatus); err != nil {
		return nil, fmt.Errorf("failed to deserialize steps_status: %w", err)
	}

	if err := json.Unmarshal([]byte(stepResultsJSON), &cp.StepResults); err != nil {
		return nil, fmt.Errorf("failed to deserialize step_results: %w", err)
	}

	if memorySnapshotJSON != "" {
		var snapshot MemorySnapshot
		if err := json.Unmarshal([]byte(memorySnapshotJSON), &snapshot); err != nil {
			return nil, fmt.Errorf("failed to deserialize memory_snapshot: %w", err)
		}
		cp.MemorySnapshot = &snapshot
	}

	if shadowFilesJSON != "" {
		if err := json.Unmarshal([]byte(shadowFilesJSON), &cp.ShadowFiles); err != nil {
			return nil, fmt.Errorf("failed to deserialize shadow_files: %w", err)
		}
	}

	if metadataJSON != "" {
		if err := json.Unmarshal([]byte(metadataJSON), &cp.Metadata); err != nil {
			return nil, fmt.Errorf("failed to deserialize metadata: %w", err)
		}
	}

	return &cp, nil
}

// ListCheckpoints returns all checkpoints for a session, ordered by timestamp
func (s *Store) ListCheckpoints(sessionID string) ([]CheckpointMetadata, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, session_id, task, timestamp, status, progress, total_cost, current_step, turn_count
		FROM checkpoints
		WHERE session_id = ?
		ORDER BY timestamp DESC
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var checkpoints []CheckpointMetadata
	for rows.Next() {
		var cp CheckpointMetadata
		var status string
		if err := rows.Scan(
			&cp.ID, &cp.SessionID, &cp.Task, &cp.Timestamp, &status,
			&cp.Progress, &cp.TotalCost, &cp.CurrentStep, &cp.TurnCount,
		); err != nil {
			return nil, err
		}
		cp.Status = CheckpointStatus(status)
		checkpoints = append(checkpoints, cp)
	}

	return checkpoints, rows.Err()
}

// GetLatestCheckpoint retrieves the most recent checkpoint for a session
func (s *Store) GetLatestCheckpoint(sessionID string) (*Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var checkpointID string
	err := s.db.QueryRow(`
		SELECT id FROM checkpoints
		WHERE session_id = ?
		ORDER BY timestamp DESC
		LIMIT 1
	`, sessionID).Scan(&checkpointID)

	if err != nil {
		return nil, fmt.Errorf("no checkpoint found for session: %w", err)
	}

	// Unlock before calling LoadCheckpoint (which will acquire its own lock)
	s.mu.RUnlock()
	defer s.mu.RLock() // Re-lock for defer unlock

	return s.LoadCheckpoint(checkpointID)
}

// UpdateCheckpointStatus updates the status of a checkpoint
func (s *Store) UpdateCheckpointStatus(id string, status CheckpointStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		UPDATE checkpoints
		SET status = ?, timestamp = ?
		WHERE id = ?
	`, string(status), time.Now(), id)

	return err
}

// DeleteCheckpoint removes a checkpoint from the database
func (s *Store) DeleteCheckpoint(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM checkpoints WHERE id = ?", id)
	return err
}

// DeleteSessionCheckpoints removes all checkpoints for a session
func (s *Store) DeleteSessionCheckpoints(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM checkpoints WHERE session_id = ?", sessionID)
	return err
}

// GetAllSessions returns a list of all unique session IDs
func (s *Store) GetAllSessions() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT DISTINCT session_id
		FROM checkpoints
		ORDER BY MAX(timestamp) DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []string
	for rows.Next() {
		var sessionID string
		if err := rows.Scan(&sessionID); err != nil {
			return nil, err
		}
		sessions = append(sessions, sessionID)
	}

	return sessions, rows.Err()
}
