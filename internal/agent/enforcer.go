package agent

import (
	"encoding/json"
	"fmt"

	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/policy"
	"github.com/david22573/codepicker/internal/tools"
)

type ApprovalRequest struct {
	Tool   string
	Args   string
	Reason string
}

type InteractionFunc func(req ApprovalRequest) bool

type PolicyEnforcer struct {
	Policy     policy.ExecutionPolicy
	Logger     logger.Logger
	Sentinel   *Sentinel
	OnApproval InteractionFunc
	// Cache tool capabilities for lookups
	ToolCaps map[string][]tools.Capability
}

func NewPolicyEnforcer(p policy.ExecutionPolicy, log logger.Logger, sentinel *Sentinel) *PolicyEnforcer {
	return &PolicyEnforcer{
		Policy:     p,
		Logger:     log,
		Sentinel:   sentinel,
		OnApproval: DefaultCLIInteraction,
		ToolCaps:   make(map[string][]tools.Capability),
	}
}

func (pe *PolicyEnforcer) RegisterTool(t tools.Tool) {
	pe.ToolCaps[t.Name()] = t.Capabilities()
}

func (pe *PolicyEnforcer) AllowTool(req ApprovalRequest) bool {
	caps, exists := pe.ToolCaps[req.Tool]
	if !exists {
		// If tool not registered (e.g. system internal), default to strict check
		pe.Logger.Warn(fmt.Sprintf("Security: Unknown tool '%s' attempted execution.", req.Tool))
		return false
	}

	// 1. Check Capabilities against Policy
	for _, cap := range caps {
		switch cap {
		case tools.CapExecute:
			if !pe.Policy.AllowShell {
				pe.Logger.Warn(fmt.Sprintf("Policy Violation: Tool '%s' requires Shell access.", req.Tool))
				return false
			}
		case tools.CapWrite:
			if !pe.Policy.AllowFileWrite {
				pe.Logger.Warn(fmt.Sprintf("Policy Violation: Tool '%s' requires Write access.", req.Tool))
				return false
			}
			// Read is usually allowed, but we could restrict it in strict modes
		}
	}

	// 2. Specific Shell Checks (Deep inspection via Sentinel)
	if hasCapability(caps, tools.CapExecute) {
		var shellArgs struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal([]byte(req.Args), &shellArgs); err == nil {
			classification := pe.Sentinel.ClassifyCommand(shellArgs.Command)

			if classification == ClassDangerous && pe.Policy.Mode != policy.LevelInteractive {
				pe.Logger.Warn(fmt.Sprintf("Security: Blocked dangerous command '%s' in %s mode", shellArgs.Command, pe.Policy.Mode))
				return false
			}

			if err := pe.Policy.ValidateCommand(shellArgs.Command); err != nil {
				pe.Logger.Warn(fmt.Sprintf("Policy Violation: %v", err))
				return false
			}
		}
	}

	// 3. Interactive Approval
	if pe.Policy.Mode == policy.LevelInteractive {
		if pe.OnApproval == nil {
			return false
		}
		return pe.OnApproval(req)
	}

	return true
}

func (pe *PolicyEnforcer) SetInteractionHandler(fn InteractionFunc) {
	pe.OnApproval = fn
}

func DefaultCLIInteraction(req ApprovalRequest) bool {
	fmt.Printf("\n⚠️  Agent Request Approval\n")
	fmt.Printf("   Tool:   \033[1m%s\033[0m\n", req.Tool)

	if len(req.Args) < 200 {
		fmt.Printf("   Args:   %s\n", req.Args)
	} else {
		fmt.Printf("   Args:   [Large JSON payload, %d bytes]\n", len(req.Args))
	}

	if req.Reason != "" {
		fmt.Printf("   Reason: %s\n", req.Reason)
	}

	fmt.Printf("   Allow? [Y/n]: ")
	var resp string
	fmt.Scanln(&resp)
	return resp == "" || resp == "y" || resp == "Y"
}

// Renamed to avoid collision with integration_test.go
func hasCapability(caps []tools.Capability, target tools.Capability) bool {
	for _, c := range caps {
		if c == target {
			return true
		}
	}
	return false
}
