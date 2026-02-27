package agent

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	domainAgent "github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/audit"
	"github.com/david22573/codepicker/domain/event"
	"github.com/david22573/codepicker/infra/llm"
	"github.com/david22573/codepicker/infra/logging"
	"github.com/david22573/codepicker/infra/prompts"
	"github.com/david22573/codepicker/infra/ratelimit"
	"github.com/david22573/codepicker/runtime"
	"go.uber.org/zap"
)

type Auditor struct {
	model       llm.Provider
	repo        domainAgent.Repository
	tools       []domainAgent.Tool
	policy      domainAgent.Policy
	logger      *logging.Logger
	costTracker *llm.CostTracker
	rateLimiter *ratelimit.ToolRateLimiter
	bus         *event.DataBus
	budget      float64
}

// NewAuditor initializes the auditor with a budget cap.
func NewAuditor(
	model llm.Provider,
	repo domainAgent.Repository,
	tools []domainAgent.Tool,
	policy domainAgent.Policy,
	logger *logging.Logger,
	costTracker *llm.CostTracker,
	rateLimiter *ratelimit.ToolRateLimiter,
	bus *event.DataBus,
	budget float64,
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
	systemPrompt, err := prompts.Render("auditor_scout", map[string]any{
		"Primer": primer,
	})
	if err != nil {
		return nil, err
	}

	scoutBudget := a.budget * runtime.Global.ScoutBudgetPercent
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

	a.logger.Info("scanning for improvements using Native Tool Calling", zap.String("phase", "scout"))
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
	systemPrompt, err := prompts.Render("auditor_comprehensive", nil)
	if err != nil {
		return nil, err
	}

	auditBudget := a.budget * runtime.Global.AuditBudgetPercent
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

	a.logger.Info("starting comprehensive analysis", zap.String("phase", "auditor"))
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