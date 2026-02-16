package agent

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/audit"
	"github.com/david22573/codepicker/domain/event"
	"github.com/david22573/codepicker/infra/llm"
	"github.com/david22573/codepicker/infra/logging"
	"github.com/david22573/codepicker/infra/ratelimit"
)

type Auditor struct {
	model       *llm.OpenRouterAdapter
	repo        agent.Repository
	tools       []agent.Tool
	policy      agent.Policy
	logger      *logging.Logger
	costTracker *llm.CostTracker
	rateLimiter *ratelimit.ToolRateLimiter
	bus         *event.DataBus
	budget      float64 // FIX: Store budget to allocate to sub-agents
}

// NewAuditor initializes the auditor with a budget cap.
func NewAuditor(
	model *llm.OpenRouterAdapter,
	repo agent.Repository,
	tools []agent.Tool,
	policy agent.Policy,
	logger *logging.Logger,
	costTracker *llm.CostTracker,
	rateLimiter *ratelimit.ToolRateLimiter,
	bus *event.DataBus,
	budget float64, // FIX: Added budget argument
) *Auditor {
	return &Auditor{
		model:       model,
		repo:        repo,
		tools:       tools,
		policy:      policy,
		logger:      logger,
		costTracker: costTracker,
		rateLimiter: rateLimiter,
		bus:         bus,
		budget:      budget,
	}
}

// SuggestImprovements scans the codebase to identify actionable code quality or security tasks.
func (a *Auditor) SuggestImprovements(ctx context.Context, primer string) ([]string, error) {
	systemPrompt := fmt.Sprintf(`%s

You are the CodePicker Scout, a specialist in identifying high-impact, low-risk code improvements.
Your goal is to scan the codebase and identify exactly 3 SAFE, ISOLATED improvements.

Focus areas:
1. Error handling (e.g., unhandled errors).
2. Code hygiene (e.g., unused variables).
3. Documentation (e.g., missing comments).
4. Simple refactors.

RULES:
1. You MUST use tools to see the code.
2. Your Final Answer must list the improvements, each starting with "TASK: ".`, primer)

	// FIX: Allocate 20% of budget to the Scout (cheap, fast scan)
	scoutBudget := a.budget * 0.20
	if scoutBudget < 0.2 {
		scoutBudget = 0.2 // Minimum floor
	}

	scout := NewReActAgent(
		a.model,
		a.tools,
		a.bus,
		a.logger,
		a.policy,
		a.costTracker,
		a.rateLimiter,
		scoutBudget,
		20,
	)

	scout.UpdateSystemPrompt(systemPrompt)

	fmt.Println("📡 [SCOUT] Scanning for improvements using Native Tool Calling...")
	result, err := scout.Run(ctx, "Analyze the current directory and suggest 3 high-quality improvements.")
	if err != nil {
		return nil, fmt.Errorf("scout scanning failed: %w", err)
	}

	var tasks []string
	lines := strings.Split(result, "\n")
	for _, line := range lines {
		clean := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(clean), "TASK:") {
			parts := strings.SplitN(clean, ":", 2)
			if len(parts) == 2 {
				tasks = append(tasks, strings.TrimSpace(parts[1]))
			}
		}
	}

	return tasks, nil
}

// RunAudit performs a comprehensive security and quality analysis.
func (a *Auditor) RunAudit(ctx context.Context, input string) (*audit.Report, error) {
	systemPrompt := `You are CodePicker-Auditor, a senior security researcher and software architect.
Your goal is to AUDIT the codebase for vulnerabilities, technical debt, and architectural drift.
STRICT READ-ONLY MODE: You cannot modify any files.
Your Final Answer MUST be a comprehensive Markdown report.`

	// FIX: Allocate 80% of budget to the Auditor (expensive, deep analysis)
	auditBudget := a.budget * 0.80
	if auditBudget < 1.0 {
		auditBudget = 1.0 // Minimum floor
	}

	auditAgent := NewReActAgent(
		a.model,
		a.tools,
		a.bus,
		a.logger,
		a.policy,
		a.costTracker,
		a.rateLimiter,
		auditBudget,
		200,
	)

	auditAgent.UpdateSystemPrompt(systemPrompt)

	fmt.Println("🔍 [AUDITOR] Starting comprehensive analysis...")
	result, err := auditAgent.Run(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("audit failed: %w", err)
	}

	reportID := fmt.Sprintf("audit-%d", time.Now().Unix())
	fileName := fmt.Sprintf("audit_report_%s.md", reportID)
	if err := os.WriteFile(fileName, []byte(result), 0644); err != nil {
		return nil, fmt.Errorf("failed to save audit report: %w", err)
	}

	return &audit.Report{
		ID:        reportID,
		Timestamp: time.Now(),
		Content:   result,
		Artifact:  fileName,
	}, nil
}
