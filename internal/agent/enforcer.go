package agent

import (
	"fmt"

	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/policy"
)

// InteractionFunc defines how we ask the human for permission.
// Returns true if approved, false if denied.
type InteractionFunc func(command, reason string) bool

type PolicyEnforcer struct {
	Policy     policy.ExecutionPolicy
	Logger     logger.Logger
	OnApproval InteractionFunc
}

func NewPolicyEnforcer(p policy.ExecutionPolicy, log logger.Logger) *PolicyEnforcer {
	return &PolicyEnforcer{
		Policy: p,
		Logger: log,
		// Default interaction: Always deny if interactive and no handler set
		OnApproval: func(c, r string) bool { return false },
	}
}

// Check verifies if a command is allowed by the current policy.
// It handles both automatic checks (whitelists) and interactive approvals.
func (pe *PolicyEnforcer) Check(command, reason string) bool {
	// 1. Static Policy Check (Allowlist/Blocklist)
	if err := pe.Policy.ValidateCommand(command); err != nil {
		pe.Logger.Warn(fmt.Sprintf("Policy Denied: %v", err))
		return false
	}

	// 2. Interactive Check (if mode requires it)
	if pe.Policy.Mode == policy.LevelInteractive {
		if pe.OnApproval == nil {
			pe.Logger.Warn("Interactive policy set but no interaction handler defined. Denying.")
			return false
		}
		return pe.OnApproval(command, reason)
	}

	// 3. Auto/Strict modes fall through here if ValidateCommand passed
	return true
}

// SetInteractionHandler allows overriding how we ask the user (CLI vs Web)
func (pe *PolicyEnforcer) SetInteractionHandler(fn InteractionFunc) {
	pe.OnApproval = fn
}

// DefaultCLIInteraction provides the standard console Y/n prompt
func DefaultCLIInteraction(command, reason string) bool {
	fmt.Printf("\n⚠️  Agent wants to run: %s\n   Reason: %s\n   Allow? [Y/n]: ", command, reason)
	var resp string
	fmt.Scanln(&resp)
	return resp == "" || resp == "y" || resp == "Y"
}
