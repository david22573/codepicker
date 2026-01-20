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
	ToolCaps   map[string][]tools.Capability
	Debug      bool
}

func NewPolicyEnforcer(p policy.ExecutionPolicy, log logger.Logger, sentinel *Sentinel, debug bool) *PolicyEnforcer {
	return &PolicyEnforcer{
		Policy:     p,
		Logger:     log,
		Sentinel:   sentinel,
		OnApproval: DefaultCLIInteraction,
		ToolCaps:   make(map[string][]tools.Capability),
		Debug:      debug,
	}
}

func (pe *PolicyEnforcer) RegisterTool(t tools.Tool) {
	pe.ToolCaps[t.Name()] = t.Capabilities()
}

func (pe *PolicyEnforcer) AllowTool(req ApprovalRequest) bool {
	if pe.Debug {
		pe.Logger.Info(fmt.Sprintf("[Policy] Checking tool: %s", req.Tool))
	}

	caps, exists := pe.ToolCaps[req.Tool]
	if !exists {
		pe.Logger.Warn(fmt.Sprintf("Security: Unknown tool '%s' attempted execution.", req.Tool))
		return false
	}

	for _, cap := range caps {
		if pe.Debug {
			pe.Logger.Info(fmt.Sprintf("[Policy] Required Capability: %s", cap))
		}
		switch cap {
		case tools.CapExecute:
			if !pe.Policy.AllowShell {
				pe.Logger.Warn("Policy Violation: Shell access required.")
				return false
			}
		case tools.CapWrite:
			if !pe.Policy.AllowFileWrite {
				pe.Logger.Warn("Policy Violation: Write access required.")
				return false
			}
		}
	}

	// [Fixed] Rename contains -> hasCapability
	if hasCapability(caps, tools.CapExecute) {
		var shellArgs struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal([]byte(req.Args), &shellArgs); err == nil {
			classification := pe.Sentinel.ClassifyCommand(shellArgs.Command)
			if pe.Debug {
				pe.Logger.Info(fmt.Sprintf("[Policy] Classification: %s", classification))
			}

			if classification == ClassDangerous && pe.Policy.Mode != policy.LevelInteractive {
				pe.Logger.Warn(fmt.Sprintf("Security: Blocked dangerous command '%s'", shellArgs.Command))
				return false
			}
			if err := pe.Policy.ValidateCommand(shellArgs.Command); err != nil {
				pe.Logger.Warn(fmt.Sprintf("Policy Violation: %v", err))
				return false
			}
		}
	}

	if pe.Policy.Mode == policy.LevelInteractive {
		if pe.OnApproval == nil {
			return false
		}
		return pe.OnApproval(req)
	}
	return true
}

func (pe *PolicyEnforcer) SetInteractionHandler(fn InteractionFunc) { pe.OnApproval = fn }

func DefaultCLIInteraction(req ApprovalRequest) bool {
	fmt.Printf("\n⚠️  Agent Request Approval\n   Tool: %s\n   Args: %s\n   Allow? [Y/n]: ", req.Tool, req.Args)
	var resp string
	fmt.Scanln(&resp)
	return resp == "" || resp == "y" || resp == "Y"
}

// [Fixed] Renamed function
func hasCapability(caps []tools.Capability, target tools.Capability) bool {
	for _, c := range caps {
		if c == target {
			return true
		}
	}
	return false
}
