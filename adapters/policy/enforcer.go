package policy

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"

	"github.com/david22573/codepicker/infra/pathutil"
	"github.com/david22573/codepicker/infra/ui"
	"github.com/david22573/codepicker/runtime"
)

type contextKey string

const TaskIDKey contextKey = "task_id"

var (
	// CLI-only fallback state
	sessionAutoApproveCLI bool

	// Daemon state
	DaemonMode bool
	mu         sync.Mutex
	sessions   = make(map[string]*SessionState)
)

type SessionState struct {
	AutoApprove bool
	ReqCh       chan ApprovalRequest
	RespCh      chan ApprovalResponse
}

type ApprovalRequest struct {
	Filename string
	Blocks   string
}

type ApprovalResponse struct {
	Action string
	Blocks string
}

func GetOrCreateSession(taskID string) *SessionState {
	mu.Lock()
	defer mu.Unlock()
	if s, exists := sessions[taskID]; exists {
		return s
	}
	s := &SessionState{
		// Buffered to prevent goroutine leaks if client disconnects
		ReqCh:  make(chan ApprovalRequest, 1),
		RespCh: make(chan ApprovalResponse, 1),
	}
	sessions[taskID] = s
	return s
}

func GetSession(taskID string) *SessionState {
	mu.Lock()
	defer mu.Unlock()
	return sessions[taskID]
}

func CleanupSession(taskID string) {
	mu.Lock()
	defer mu.Unlock()
	delete(sessions, taskID)
}

type Enforcer struct {
	config           PolicySchema
	readOnly         bool
	forbiddenRegex   []*regexp.Regexp
	commandWhitelist map[string]bool
}

func NewEnforcer(config PolicySchema, readOnly bool) *Enforcer {
	var regexList []*regexp.Regexp
	for _, keyword := range config.ForbiddenKeywords {
		if len(keyword) == 0 {
			continue
		}

		cleanKeyword := strings.ReplaceAll(regexp.QuoteMeta(keyword), " ", `\s+`)
		pattern := `(?i)`
		if isWordChar(keyword[0]) {
			pattern += `\b`
		}
		pattern += cleanKeyword
		if isWordChar(keyword[len(keyword)-1]) {
			pattern += `\b`
		}

		if regex, err := regexp.Compile(pattern); err == nil {
			regexList = append(regexList, regex)
		}
	}

	whitelist := map[string]bool{
		"go":   true,
		"git":  true,
		"ls":   true,
		"cat":  true,
		"grep": true,
		"make": true,
	}

	return &Enforcer{
		config:           config,
		readOnly:         readOnly,
		forbiddenRegex:   regexList,
		commandWhitelist: whitelist,
	}
}

func isWordChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

func (e *Enforcer) Mode() string {
	if e.readOnly {
		return "guarded-readonly"
	}
	return "guarded-active"
}

func (e *Enforcer) CanExecute(toolName string, args string) (bool, string) {
	if e.readOnly {
		if toolName == "write_file" || toolName == "edit_file" || toolName == "run_cmd" {
			return false, "BLOCKED: Side-effects are disabled in READ-ONLY mode."
		}
	}

	for _, regex := range e.forbiddenRegex {
		if regex.MatchString(args) {
			return false, fmt.Sprintf("BLOCKED: Forbidden pattern detected: %s", regex.String())
		}
	}

	switch toolName {
	case "run_cmd":
		return e.validateCommand(args)
	case "write_file", "edit_file", "read_file":
		return e.validateFileSystemAccess(toolName, args)
	}

	return true, ""
}

func (e *Enforcer) validateCommand(args string) (bool, string) {
	var input struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return false, "BLOCKED: Invalid JSON for run_cmd"
	}

	cleanCmd := strings.TrimSpace(input.Command)
	if cleanCmd == "" {
		return false, "BLOCKED: Command cannot be empty"
	}

	cmdToCheck := strings.ReplaceAll(cleanCmd, "./...", "")
	if strings.Contains(cmdToCheck, "..") {
		return false, "BLOCKED: Path traversal (..) detected in command"
	}

	parts := strings.Fields(cleanCmd)
	if len(parts) == 0 {
		return false, "BLOCKED: Malformed command"
	}

	baseCmd := parts[0]
	if !e.commandWhitelist[baseCmd] {
		return false, fmt.Sprintf("BLOCKED: Command '%s' is not in the whitelist", baseCmd)
	}

	dangerous := []string{"|", ">", "&&", "||", ";", "`", "$(", "<", ">>", "&"}
	for _, d := range dangerous {
		if strings.Contains(cleanCmd, d) {
			return false, fmt.Sprintf("BLOCKED: Dangerous shell operator detected: %s", d)
		}
	}

	return true, ""
}

func (e *Enforcer) validateFileSystemAccess(toolName, args string) (bool, string) {
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return false, fmt.Sprintf("BLOCKED: Invalid JSON for %s", toolName)
	}

	if input.Path == "" {
		return false, "BLOCKED: Path argument is missing"
	}

	if _, err := pathutil.Clean(input.Path); err != nil {
		return false, fmt.Sprintf("BLOCKED: %s", err.Error())
	}

	return true, ""
}

// --- Phase 3.2 / 5.1: Interactive & Async Approval Gate ---

func AskApproval(ctx context.Context, filename, blocks string) (string, string) {
	if runtime.Global.Mode == runtime.ModeHardenedCI {
		ui.PrintWarning("ModeHardenedCI active: Auto-rejecting interactive edit prompt.")
		return "n", blocks
	}

	taskID, hasTask := ctx.Value(TaskIDKey).(string)

	if DaemonMode {
		if !hasTask {
			// Fail safe if daemon mode is improperly wired
			return "n", blocks
		}

		session := GetOrCreateSession(taskID)
		if session.AutoApprove {
			return "y", blocks
		}

		select {
		case session.ReqCh <- ApprovalRequest{Filename: filename, Blocks: blocks}:
		case <-ctx.Done():
			return "n", blocks
		}

		select {
		case resp := <-session.RespCh:
			if resp.Action == "s" {
				session.AutoApprove = true
				return "y", resp.Blocks
			}
			return resp.Action, resp.Blocks
		case <-ctx.Done():
			return "n", blocks
		}
	}

	// CLI Mode
	if sessionAutoApproveCLI {
		return "y", blocks
	}

	fmt.Println(ui.RenderDiff(filename, blocks))

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print(ui.InfoStyle.Render("Apply this change? [y/n/e(dit)/s(kip all)]: "))
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))

		switch input {
		case "y":
			return "y", blocks
		case "n":
			return "n", blocks
		case "s":
			sessionAutoApproveCLI = true
			return "y", blocks
		case "e":
			edited, err := openEditor(blocks)
			if err != nil {
				ui.PrintError("Editor failed: " + err.Error())
				continue
			}
			return "y", edited
		default:
			ui.PrintWarning("Invalid option. Choose y, n, e, or s.")
		}
	}
}

func openEditor(content string) (string, error) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "nano"
	}
	tmp, err := os.CreateTemp("", "codepicker-edit-*.txt")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())

	tmp.WriteString(content)
	tmp.Close()

	cmd := exec.Command(editor, tmp.Name())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}

	edited, err := os.ReadFile(tmp.Name())
	return string(edited), err
}
