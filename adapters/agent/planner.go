package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/task"
	"github.com/david22573/codepicker/infra/llm"
)

// Planner generates execution plans for complex tasks using structured JSON output.
type Planner struct {
	model llm.StructuredLLM
}

// NewPlanner creates a new planner with the structured LLM adapter.
func NewPlanner(client agent.LLMClient) *Planner {
	return &Planner{
		model: llm.NewStructuredAdapter(client),
	}
}

// CreatePlan creates a step-by-step plan for the given task.
func (p *Planner) CreatePlan(ctx context.Context, taskDesc, fileContext, primer string) (*task.Plan, error) {
	// Detailed prompt to guide the LLM toward a high-quality JSON response
	systemPrompt := fmt.Sprintf(`You are CodePicker Planner. Your job is to create a detailed execution plan.

PROJECT STRUCTURE:
%s

USER TASK: %s

CRITICAL PLANNING REQUIREMENTS:
1. Break down the task into small, isolated steps (1-3 files per step).
2. Each step must be independently executable.
3. Order steps by dependency (read before write, imports before usage, etc.).
4. Instructions must be ACTIONABLE - tell the executor WHAT TO DO.

JSON OUTPUT FORMAT:
You must output a single JSON object matching this schema:
{
  "reasoning": "Explanation of your strategy...",
  "steps": [
    {
      "description": "Short summary of step",
      "instruction": "Detailed directive for the agent",
      "files": ["relative/path/to/file.go"]
    }
  ]
}
`, primer, taskDesc)

	userPrompt := fmt.Sprintf("Create a plan for: %s", taskDesc)

	// Container for the structured response
	var schema task.PlanSchema

	if err := p.model.ChatJSON(ctx, systemPrompt, userPrompt, &schema); err != nil {
		return nil, fmt.Errorf("failed to generate structured plan: %w", err)
	}

	return p.convertSchemaToPlan(schema, taskDesc)
}

// OptimizePlan uses AI to refine an existing plan based on feedback.
func (p *Planner) OptimizePlan(ctx context.Context, plan *task.Plan, feedback string) (*task.Plan, error) {
	// Placeholder for future implementation (Reflexion loop)
	return plan, nil
}

// convertSchemaToPlan transforms the JSON schema into the internal domain model.
func (p *Planner) convertSchemaToPlan(schema task.PlanSchema, taskDesc string) (*task.Plan, error) {
	planID := fmt.Sprintf("plan-%d", time.Now().UnixNano())
	plan := task.NewPlan(planID, taskDesc, schema.Reasoning)

	if len(schema.Steps) == 0 {
		return nil, fmt.Errorf("generated plan contains no steps")
	}

	for _, s := range schema.Steps {
		cleanedFiles := cleanFilePaths(s.Files)
		plan.AddStep(s.Description, s.Instruction, cleanedFiles)
	}

	return plan, nil
}

// cleanFilePaths sanitizes the file list returned by the LLM.
func cleanFilePaths(paths []string) []string {
	var cleaned []string
	seen := make(map[string]bool)

	for _, path := range paths {
		path = strings.Trim(path, `"'[] `)
		if path == "" || strings.HasSuffix(path, "/") {
			continue
		}
		path = filepath.Clean(path)

		// Deduplicate and filter obvious non-files
		if !seen[path] && looksLikeFile(path) {
			cleaned = append(cleaned, path)
			seen[path] = true
		}
	}
	return cleaned
}

// looksLikeFile checks if the path has an extension or is a known config file.
func looksLikeFile(path string) bool {
	base := filepath.Base(path)
	if strings.Contains(base, ".") {
		return true
	}
	knownFiles := []string{"Makefile", "Dockerfile", "go.mod", "go.sum", "codepicker.yaml", "LICENSE"}
	for _, known := range knownFiles {
		if strings.EqualFold(base, known) {
			return true
		}
	}
	return false
}
