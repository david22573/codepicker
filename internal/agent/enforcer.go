package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/policy"
	"github.com/david22573/codepicker/internal/tools"
)

type ApprovalRequest struct {
	Tool   string
	Args   string
	Reason string
}

// ApprovalResponse allows the UI to return more than just a boolean
type ApprovalResponse struct {
	Approved     bool
	SessionScope bool // If true, approve this capability for the rest of the session
}

type InteractionFunc func(req ApprovalRequest) ApprovalResponse

type SessionApprovals struct {
	AllowWrite bool
	AllowExec  bool
}

type PolicyEnforcer struct {
	Policy     policy.ExecutionPolicy
	Logger     logger.Logger
	Sentinel   *Sentinel
	OnApproval InteractionFunc
	ToolCaps   map[string][]tools.Capability
	Session    SessionApprovals
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

	// 1. PHASE 1 FIX: Silent Read-Only
	// If the tool is purely read-only (e.g. search_code, read_file), we NEVER ask for permission
	// unless we are in a hyper-strict mode (which we aren't implementing yet).
	if tools.IsReadOnly(caps) {
		return true
	}

	// 2. Check Hard Policy blocks (e.g. Shell disabled in Batch mode)
	for _, cap := range caps {
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

	// 3. Sentinel Checks (Command Injection / Dangerous Patterns)
	if tools.HasCapability(caps, tools.CapExecute) {
		var shellArgs struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal([]byte(req.Args), &shellArgs); err == nil {
			classification := pe.Sentinel.ClassifyCommand(shellArgs.Command)

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

	// 4. PHASE 2 FIX: Session Caching
	// If we have already approved this capability for this session, skip the prompt.
	if pe.Policy.Mode == policy.LevelInteractive {
		if tools.HasCapability(caps, tools.CapWrite) && pe.Session.AllowWrite {
			return true
		}
		if tools.HasCapability(caps, tools.CapExecute) && pe.Session.AllowExec {
			// Even if exec is allowed, we might want to prompt for *new* dangerous commands
			// But for now, we follow the roadmap: Trust the session.
			return true
		}

		if pe.OnApproval == nil {
			return false
		}

		// 5. Ask User
		resp := pe.OnApproval(req)

		if resp.Approved && resp.SessionScope {
			if tools.HasCapability(caps, tools.CapWrite) {
				pe.Session.AllowWrite = true
				pe.Logger.Info("🔓 Write access granted for remainder of session.")
			}
			if tools.HasCapability(caps, tools.CapExecute) {
				pe.Session.AllowExec = true
				pe.Logger.Info("🔓 Shell access granted for remainder of session.")
			}
		}

		return resp.Approved
	}

	return true
}

func (pe *PolicyEnforcer) SetInteractionHandler(fn InteractionFunc) { pe.OnApproval = fn }

// DefaultCLIInteraction updated to support "Always" options
func DefaultCLIInteraction(req ApprovalRequest) ApprovalResponse {
	// Simple heuristic to pretty-print args
	displayArgs := req.Args
	if len(displayArgs) > 100 {
		displayArgs = displayArgs[:97] + "..."
	}

	fmt.Printf("\n⚠️  Agent Request: \033[1m%s\033[0m\n   Args: %s\n", req.Tool, displayArgs)
	fmt.Printf("   [y] Yes  [n] No  [a] Always allow (Session)\n   Action? ")

	var resp string
	fmt.Scanln(&resp)
	resp = strings.ToLower(strings.TrimSpace(resp))

	if resp == "a" || resp == "all" {
		return ApprovalResponse{Approved: true, SessionScope: true}
	}

	approved := resp == "" || resp == "y" || resp == "yes"
	return ApprovalResponse{Approved: approved, SessionScope: false}
}
