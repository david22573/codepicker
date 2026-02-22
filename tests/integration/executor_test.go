package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/david22573/codepicker/adapters/agent"
	"github.com/david22573/codepicker/domain/task"
	"github.com/david22573/codepicker/infra/fs"
	"github.com/david22573/codepicker/infra/logging"
)

// MockAgent implements the domain/agent.Agent interface
type MockAgent struct {
	Response string
	Error    error
}

func (m *MockAgent) Run(ctx context.Context, input string) (string, error) {
	return m.Response, m.Error
}

// MockRepo implements agent.Repository
type MockRepo struct{}

func (m *MockRepo) SavePlan(ctx context.Context, plan *task.Plan) error { return nil }
func (m *MockRepo) GetPlan(ctx context.Context, id string) (*task.Plan, error) { return nil, nil }
func (m *MockRepo) ListPlans(ctx context.Context, limit int) ([]agent.PlanSummary, error) { return nil, nil }
func (m *MockRepo) DeletePlan(ctx context.Context, id string) error { return nil }
func (m *MockRepo) SaveExecution(ctx context.Context, exec *agent.Execution) error { return nil }
func (m *MockRepo) GetExecution(ctx context.Context, id string) (*agent.Execution, error) { return nil, nil }
func (m *MockRepo) ListExecutions(ctx context.Context, limit int) ([]agent.ExecutionSummary, error) { return nil, nil }

func TestPlanExecutor_Integration(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "executor_integration_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logger, _ := logging.NewLogger("development", false)
	workspaceMgr := fs.NewWorkspaceManager(tempDir)
	shadowMgr := fs.NewShadowManager(tempDir, false)
	mockRepo := &MockRepo{}
	
	mockWorker := &MockAgent{
		Response: "Final Answer: completed task successfully",
		Error:    nil,
	}

	executor := agent.NewPlanExecutor(mockWorker, mockRepo, workspaceMgr, shadowMgr, logger)
	executor.SetAutoConfirm(true) // Non-interactive

	testPlan := &task.Plan{
		ID: "test-plan-1",
		Steps: []task.Step{
			{
				ID:          1,
				Description: "Test step",
				Instruction: "Create a file",
				Files:       []string{"main.go"},
				Status:      task.StatusPending,
			},
		},
	}

	// Simulate agent making a change in the shadow FS during its execution
	_, _ = shadowMgr.Write("main.go", []byte("package main\n"))

	err = executor.Execute(context.Background(), testPlan)
	if err != nil {
		t.Fatalf("executor failed: %v", err)
	}

	if testPlan.Status != task.StatusCompleted {
		t.Errorf("expected plan status %s, got %s", task.StatusCompleted, testPlan.Status)
	}

	// Verify the simulated shadow change was committed to the workspace
	realPath := filepath.Join(tempDir, "main.go")
	if _, err := os.Stat(realPath); os.IsNotExist(err) {
		t.Errorf("expected committed file main.go to exist, but it does not")
	}
}