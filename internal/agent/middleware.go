package agent

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/david22573/codepicker/internal/logger"
)

// FormattingMiddleware automatically formats code after it is written
type FormattingMiddleware struct {
	ShadowRoot string
	Logger     logger.Logger
}

func NewFormattingMiddleware(shadowRoot string, log logger.Logger) *FormattingMiddleware {
	return &FormattingMiddleware{
		ShadowRoot: shadowRoot,
		Logger:     log,
	}
}

func (m *FormattingMiddleware) BeforeExecute(toolName string, args string) error {
	return nil
}

func (m *FormattingMiddleware) AfterExecute(toolName string, result string) error {
	// Only run on file writes
	if toolName != "write_shadow_file" {
		return nil
	}

	// We need to parse the args to find the file path.
	// Since AfterExecute only gets the result string, we assume the tool logic
	// succeeded. We'll re-parse the arguments from the last execution or
	// rely on the fact that we can't easily get args here without state.
	//
	// Better approach for V2: Pass args to AfterExecute.
	// For now, we will assume we can't see the args in AfterExecute easily
	// without changing the interface, OR we scan the result if it contains the path.
	//
	// Let's look at the write_shadow_file output: "Changes written to shadow file: <path>"

	if strings.HasPrefix(result, "Changes written to shadow file: ") {
		path := strings.TrimPrefix(result, "Changes written to shadow file: ")

		// If it's a Go file, format it
		if strings.HasSuffix(path, ".go") {
			m.Logger.Debug(fmt.Sprintf("✨ Auto-formatting %s", filepath.Base(path)))

			cmd := exec.Command("gofmt", "-w", path)
			if out, err := cmd.CombinedOutput(); err != nil {
				m.Logger.Warn(fmt.Sprintf("Auto-format failed: %v %s", err, out))
			}
		}
	}

	return nil
}

// SafetyLogMiddleware provides a high-level audit log of actions
type SafetyLogMiddleware struct {
	Logger logger.Logger
}

func NewSafetyLogMiddleware(log logger.Logger) *SafetyLogMiddleware {
	return &SafetyLogMiddleware{Logger: log}
}

func (m *SafetyLogMiddleware) BeforeExecute(toolName string, args string) error {
	// Mask potential secrets in args if necessary
	m.Logger.Info(fmt.Sprintf("🛡️  [AUDIT] Action: %s | TS: %s", toolName, time.Now().Format(time.RFC3339)))

	// Example: Block massive file writes in arguments (simple heuristic)
	if toolName == "write_shadow_file" {
		if len(args) > 500000 {
			return fmt.Errorf("payload too large for safety middleware check")
		}
	}
	return nil
}

func (m *SafetyLogMiddleware) AfterExecute(toolName string, result string) error {
	return nil
}
