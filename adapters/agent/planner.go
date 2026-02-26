package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	domainAgent "github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/task"
	"github.com/david22573/codepicker/infra/llm"
	"github.com/david22573/codepicker/infra/pathutil"
	"github.com/david22573/codepicker/infra/prompts"
)

type Planner struct {
	model llm.StructuredLLM
}

func NewPlanner(client domainAgent.LLMClient) *Planner {
	return &Planner{
		model: llm.NewStructuredAdapter(client),
	}
}

func (p *Planner) CreatePlan(ctx context.Context, taskDesc, fileContext, primer string) (*task.Plan, error) {
	systemPrompt, err := prompts.Render("planner_system", map[string]any{
		"ProjectStructure": primer,
		"UserTask":         taskDesc,
	})
	if err != nil {
		return nil, err
	}

	userPrompt := fmt.Sprintf("Create a plan for: %s", taskDesc)

	var schema task.PlanSchema

	if err := p.model.ChatJSON(ctx, systemPrompt, userPrompt, &schema); err != nil {
		return nil, fmt.Errorf("failed to generate structured plan: %w", err)
	}

	return p.convertSchemaToPlan(schema, taskDesc)
}

func (p *Planner) OptimizePlan(ctx context.Context, plan *task.Plan, feedback string) (*task.Plan, error) {
	systemPrompt, err := prompts.Render("planner_optimize_system", nil)
	if err != nil {
		return nil, err
	}

	userPrompt := fmt.Sprintf("<original_task>\n%s\n</original_task>\n\n<feedback>\n%s\n</feedback>\n\nPlease provide the optimized plan.", plan.OriginalTask, feedback)

	var schema task.PlanSchema

	if err := p.model.ChatJSON(ctx, systemPrompt, userPrompt, &schema); err != nil {
		return nil, fmt.Errorf("failed to generate optimized plan: %w", err)
	}

	optimizedPlan, err := p.convertSchemaToPlan(schema, plan.OriginalTask)
	if err != nil {
		return nil, err
	}

	optimizedPlan.ID = plan.ID
	return optimizedPlan, nil
}

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

func cleanFilePaths(paths []string) []string {
	var cleaned []string
	seen := make(map[string]bool)

	for _, path := range paths {
		path = strings.Trim(path, `"'[] `)
		if path == "" || strings.HasSuffix(path, "/") {
			continue
		}

		clean, err := pathutil.Clean(path)
		if err != nil {
			continue
		}

		if !seen[clean] && looksLikeFile(clean) {
			cleaned = append(cleaned, clean)
			seen[clean] = true
		}
	}
	return cleaned
}

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