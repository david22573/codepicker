package agent

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/audit"
	"github.com/david22573/codepicker/infra/llm"
	"github.com/david22573/codepicker/infra/logging"
)

type Auditor struct {
	model       agent.LLMClient
	repo        agent.Repository
	tools       []agent.Tool
	policy      agent.Policy
	logger      *logging.Logger
	costTracker *llm.CostTracker
}

func NewAuditor(
	model agent.LLMClient,
	repo agent.Repository,
	tools []agent.Tool,
	policy agent.Policy,
	logger *logging.Logger,
	costTracker *llm.CostTracker,
) *Auditor {
	return &Auditor{
		model:       model,
		repo:        repo,
		tools:       tools,
		policy:      policy,
		logger:      logger,
		costTracker: costTracker,
	}
}

// SuggestImprovements scans the codebase and returns a list of actionable tasks.
// UPDATED: Now accepts a 'primer' string to give the agent immediate context.
func (a *Auditor) SuggestImprovements(ctx context.Context, primer string) ([]string, error) {
	// 1. Construct Read-Only Tools
	toolDescs := ""
	toolMap := make(map[string]agent.Tool)
	for _, t := range a.tools {
		toolMap[t.Name()] = t
		toolDescs += fmt.Sprintf("- %s: %s\n", t.Name(), t.Description())
	}

	// 2. Strict System Prompt with Primer
	systemPrompt := fmt.Sprintf(`%s

You are the CodePicker Scout.
Your goal is to scan the codebase and identify 3 SAFE, ISOLATED improvements.
Focus on: Error handling, unused variables, simple refactors, or documentation.

AVAILABLE TOOLS:
%s

RULES:
1. You MUST use tools to see the code. Do not guess.
2. You MUST follow the ReAct format exactly.
3. Your Final Answer must ONLY be the list of tasks.

FORMAT EXAMPLE (Follow this exactly):
Thought: I need to see the file structure.
Action: list_files
Input: {"path": "."}
(System adds Observation...)
Thought: I see main.go. I should check it for errors.
Action: read_file
Input: {"path": "main.go"}
...
Final Answer:
TASK: Fix unhandled error in main.go
TASK: Remove unused import in adapters/parser.go

Begin.`, primer, toolDescs)

	// 3. Run the Agent
	scout := &ReActAgent{
		model:       a.model,
		tools:       toolMap,
		policy:      a.policy, // Strict Read-Only
		repo:        a.repo,
		sysMsg:      systemPrompt,
		maxTurn:     8,
		logger:      a.logger,      // FIX: Injected Logger
		costTracker: a.costTracker, // FIX: Injected CostTracker
	}

	fmt.Println("📡 [SCOUT] Scanning for improvements...")
	result, err := scout.Run(ctx, "Find 3 safe improvements in the current directory.")
	if err != nil {
		return nil, err
	}

	// 4. Parse the output into a slice
	var tasks []string
	lines := strings.Split(result, "\n")
	for _, line := range lines {
		clean := strings.TrimSpace(line)
		// We look for lines starting with TASK: (case insensitive to be safe)
		if strings.HasPrefix(strings.ToUpper(clean), "TASK:") {
			// Extract the text after "TASK:"
			parts := strings.SplitN(clean, ":", 2)
			if len(parts) == 2 {
				tasks = append(tasks, strings.TrimSpace(parts[1]))
			}
		}
	}

	return tasks, nil
}

// RunAudit performs the detailed security/quality analysis.
func (a *Auditor) RunAudit(ctx context.Context, input string) (*audit.Report, error) {
	// 1. Construct the Auditor Persona
	toolDescs := ""
	toolMap := make(map[string]agent.Tool)
	for _, t := range a.tools {
		toolMap[t.Name()] = t
		toolDescs += fmt.Sprintf("- %s: %s\n", t.Name(), t.Description())
	}

	systemPrompt := fmt.Sprintf(`You are CodePicker-Auditor, a senior security and code quality specialist.
Your goal is to AUDIT the codebase based on the user's request.
You are running in STRICT READ-ONLY MODE. You cannot modify files.

PROCESS:
1. Explore the codebase using available read tools to understand the context.
2. Identify bugs, security vulnerabilities, or architectural issues.
3. Provide a detailed Markdown report as your Final Answer.

AVAILABLE TOOLS:
%s

FORMAT:
Thought: <reasoning>
Action: <tool_name>
Input: <json_args>

Begin.`, toolDescs)

	// 2. Create an Ephemeral Agent for this Audit
	auditAgent := &ReActAgent{
		model:       a.model,
		tools:       toolMap,
		policy:      a.policy, // This must be the ReadOnly policy
		repo:        a.repo,
		sysMsg:      systemPrompt,
		maxTurn:     10,
		logger:      a.logger,      // FIX: Injected Logger
		costTracker: a.costTracker, // FIX: Injected CostTracker
	}

	// 3. Run the Agent
	fmt.Println("🔍 Auditor starting analysis...")
	result, err := auditAgent.Run(ctx, input)
	if err != nil {
		return nil, err
	}

	// 4. Generate Artifact
	reportID := fmt.Sprintf("audit-%d", time.Now().Unix())
	fileName := fmt.Sprintf("audit_report_%s.md", reportID)
	if err := os.WriteFile(fileName, []byte(result), 0644); err != nil {
		return nil, fmt.Errorf("failed to save audit artifact: %w", err)
	}

	return &audit.Report{
		ID:        reportID,
		Timestamp: time.Now(),
		Content:   result,
		Artifact:  fileName,
	}, nil
}
