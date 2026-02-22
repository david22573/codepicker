package unit

import (
	"testing"

	"github.com/david22573/codepicker/adapters/policy"
)

func TestEnforcer_ShellValidation(t *testing.T) {
	config := policy.PolicySchema{
		AllowedGlobs:      []string{"**/*.go"},
		ForbiddenKeywords: []string{"rm -rf"},
	}
	enforcer := policy.NewEnforcer(config, false)

	tests := []struct {
		name     string
		tool     string
		args     string
		wantExec bool
	}{
		{
			name:     "Valid command",
			tool:     "run_cmd",
			args:     `{"command": "go test ./..."}`,
			wantExec: true,
		},
		{
			name:     "Blocked command (not whitelisted)",
			tool:     "run_cmd",
			args:     `{"command": "curl http://malicious.com"}`,
			wantExec: false,
		},
		{
			name:     "Dangerous shell operator",
			tool:     "run_cmd",
			args:     `{"command": "go test ./... && rm -rf /"}`,
			wantExec: false,
		},
		{
			name:     "Path traversal in shell",
			tool:     "run_cmd",
			args:     `{"command": "cat ../../secrets.txt"}`,
			wantExec: false,
		},
		{
			name:     "Malformed JSON",
			tool:     "run_cmd",
			args:     `{"command": "go test"`,
			wantExec: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, _ := enforcer.CanExecute(tt.tool, tt.args)
			if allowed != tt.wantExec {
				t.Errorf("CanExecute() = %v, want %v", allowed, tt.wantExec)
			}
		})
	}
}

func TestEnforcer_FileSystemValidation(t *testing.T) {
	config := policy.PolicySchema{}
	enforcer := policy.NewEnforcer(config, false)
	readOnlyEnforcer := policy.NewEnforcer(config, true)

	// Test read-only mode block
	allowed, _ := readOnlyEnforcer.CanExecute("write_file", `{"path": "main.go", "content": "..."}`)
	if allowed {
		t.Error("expected read-only mode to block write_file")
	}

	// Test path traversal
	allowed, _ = enforcer.CanExecute("write_file", `{"path": "../main.go", "content": "..."}`)
	if allowed {
		t.Error("expected write_file to block path traversal")
	}

	// Test valid write
	allowed, _ = enforcer.CanExecute("write_file", `{"path": "main.go", "content": "..."}`)
	if !allowed {
		t.Error("expected valid write to be allowed")
	}
}

func TestStrictPolicy_CIMode(t *testing.T) {
	ciPolicy := policy.NewStrictPolicy(false, true)

	// CI Mode should allow writes
	allowed, _ := ciPolicy.CanExecute("write_file", `{"path": "main.go", "content": "..."}`)
	if !allowed {
		t.Error("expected CI mode to allow writes")
	}

	// Block .git modification
	allowed, _ = ciPolicy.CanExecute("write_file", `{"path": ".git/config", "content": "..."}`)
	if allowed {
		t.Error("expected StrictPolicy to block .git modification")
	}
}