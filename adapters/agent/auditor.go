package agent

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/audit"
)

type Auditor struct {
	model  agent.LLMClient
	repo   agent.Repository
	tools  []agent.Tool
	policy agent.Policy
}

func NewAuditor(model agent.LLMClient, repo agent.Repository, tools []agent.Tool, policy agent.Policy) *Auditor {
	return &Auditor{
		model:  model,
		repo:   repo,
		tools:  tools,
		policy: policy,
	}
}

func (a *Auditor) RunAudit(ctx context.Context, input string) (*audit.Report, error) {
	// 1. Construct the Auditor Persona
	// We override the default ReAct system prompt here to focus on analysis
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
	// We manually construct the ReActAgent struct to inject the custom prompt
	auditAgent := &ReActAgent{
		model:   a.model,
		tools:   toolMap,
		policy:  a.policy, // This must be the ReadOnly policy
		repo:    a.repo,
		sysMsg:  systemPrompt,
		maxTurn: 10, // Audits shouldn't loop forever
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
