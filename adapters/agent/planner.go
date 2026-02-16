package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/task"
)

// Planner generates execution plans for complex tasks.
type Planner struct {
	agent agent.Agent
}

// NewPlanner creates a new planner with the given agent.
func NewPlanner(agent agent.Agent) *Planner {
	return &Planner{
		agent: agent,
	}
}

// CreatePlan creates a step-by-step plan for the given task.
func (p *Planner) CreatePlan(ctx context.Context, taskDesc, fileContext, primer string) (*task.Plan, error) {
	// 🔥 IMPROVED: More explicit planning prompt that ensures executor gets actionable instructions
	prompt := fmt.Sprintf(`You are CodePicker Planner. Your job is to create a detailed execution plan.

PROJECT STRUCTURE:
%s

USER TASK: %s

Create a step-by-step plan that will be executed by an autonomous agent. Each step will be given to a worker agent that has filesystem tools.

CRITICAL PLANNING REQUIREMENTS:
1. Break down the task into small, isolated steps (1-3 files per step)
2. Each step must be independently executable
3. Order steps by dependency (read before write, imports before usage, etc.)
4. Instructions must be ACTIONABLE, not descriptive - tell the executor WHAT TO DO, not what needs to happen

MANDATORY FORMAT FOR EACH STEP:
STEP N: [One-sentence description of what this step accomplishes]
FILES: [comma-separated relative file paths that will be modified]
INSTRUCTION: [Clear directive telling the executor exactly what to do with these files]

INSTRUCTION WRITING GUIDE:
✅ GOOD Instructions (actionable directives):
- "Add slog import and replace all log.Printf calls with slog.Info calls"
- "Update the NewService function to accept *slog.Logger as the second parameter"
- "Remove the unused oldLogger variable and its initialization"

❌ BAD Instructions (vague descriptions):
- "The logger needs to be updated"
- "Fix the logging implementation"
- "Make sure slog is used properly"

EXAMPLE PLAN:
STEP 1: Update logger imports
FILES: internal/reddit/service.go
INSTRUCTION: Replace "log" import with "log/slog" and update all log.Printf calls to use slog.Info with key-value pairs.

STEP 2: Update service constructor
FILES: internal/reddit/service.go
INSTRUCTION: Modify the NewService function signature to accept logger *slog.Logger as second parameter and remove the old log.New initialization.

STEP 3: Update router setup
FILES: cmd/server/main.go
INSTRUCTION: Pass slog.Default() as the logger argument when calling reddit.NewService.

Final Answer: Created plan with 3 steps.

NOW CREATE YOUR PLAN:`, primer, taskDesc)

	response, err := p.agent.Run(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("plan generation failed: %w", err)
	}

	return p.parsePlan(response, taskDesc)
}

func (p *Planner) OptimizePlan(ctx context.Context, plan *task.Plan, feedback string) (*task.Plan, error) {
	return plan, nil
}

// parsePlan extracts structured plan from agent response using resilient parsing that handles markdown.
func (p *Planner) parsePlan(response, taskDesc string) (*task.Plan, error) {
	lines := strings.Split(response, "\n")

	planID := fmt.Sprintf("plan-%d", time.Now().UnixNano())
	plan := task.NewPlan(planID, taskDesc, "AI-generated plan")

	var currentDesc, currentInst string
	var currentFiles []string

	// Original regex patterns (case insensitive)
	stepRegex := regexp.MustCompile(`(?i)^STEP\s*\d+:?\s*(.*)`)
	filesRegex := regexp.MustCompile(`(?i)^FILES?:?\s*(.*)`)
	instRegex := regexp.MustCompile(`(?i)^INSTRUCTION:?\s*(.*)`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == "---" {
			continue
		}

		// 🔥 FIX: Clean markdown formatting BEFORE regex matching
		cleanLine := line
		cleanLine = strings.TrimPrefix(cleanLine, "**")
		cleanLine = strings.TrimSuffix(cleanLine, "**")
		cleanLine = strings.Trim(cleanLine, "`")

		if matches := stepRegex.FindStringSubmatch(cleanLine); len(matches) > 1 {
			// Save previous step if it exists
			if currentDesc != "" {
				p.addStepToPlan(plan, currentDesc, currentInst, currentFiles)
			}
			// Reset for new step
			currentDesc = strings.TrimSpace(matches[1])
			currentDesc = strings.Trim(currentDesc, "*`\"")
			currentFiles = []string{}
			currentInst = ""
			continue
		}

		if matches := filesRegex.FindStringSubmatch(cleanLine); len(matches) > 1 {
			filesStr := strings.Trim(matches[1], " []`\"*")
			parts := strings.Split(filesStr, ",")
			for _, f := range parts {
				cleanF := strings.TrimSpace(f)
				cleanF = strings.Trim(cleanF, "`\"'*")
				if cleanF != "" {
					currentFiles = append(currentFiles, cleanF)
				}
			}
			continue
		}

		if matches := instRegex.FindStringSubmatch(cleanLine); len(matches) > 1 {
			currentInst = strings.TrimSpace(matches[1])
			currentInst = strings.Trim(currentInst, "`*\"")
		}
	}

	// Add the final step
	if currentDesc != "" {
		p.addStepToPlan(plan, currentDesc, currentInst, currentFiles)
	}

	if len(plan.Steps) == 0 {
		return nil, fmt.Errorf("no valid steps found in plan. LLM output was: %s", response)
	}

	return plan, nil
}

func (p *Planner) addStepToPlan(plan *task.Plan, desc, inst string, files []string) {
	cleaned := cleanFilePaths(files)
	// Even if files are empty, we allow the step if instruction exists (it might be a shell task)
	plan.AddStep(desc, inst, cleaned)
}

func cleanFilePaths(paths []string) []string {
	var cleaned []string
	for _, path := range paths {
		path = strings.Trim(path, `"'[] `)
		if path == "" || strings.HasSuffix(path, "/") {
			continue
		}
		path = filepath.Clean(path)
		if looksLikeFile(path) {
			cleaned = append(cleaned, path)
		}
	}
	return cleaned
}

func looksLikeFile(path string) bool {
	base := filepath.Base(path)
	if strings.Contains(base, ".") {
		return true
	}
	knownFiles := []string{"Makefile", "Dockerfile", "go.mod", "go.sum", "codepicker.yaml"}
	for _, known := range knownFiles {
		if strings.EqualFold(base, known) {
			return true
		}
	}
	return false
}
