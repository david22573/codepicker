package unit

import (
	"testing"

	"github.com/david22573/codepicker/infra/pathutil"
)

func TestClean(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      string
		expectErr bool
	}{
		{
			name:      "Valid relative path",
			input:     "cmd/agent.go",
			want:      "cmd/agent.go",
			expectErr: false,
		},
		{
			name:      "Empty path",
			input:     "",
			want:      "",
			expectErr: true,
		},
		{
			name:      "Absolute path blocked",
			input:     "/etc/passwd",
			want:      "",
			expectErr: true,
		},
		{
			name:      "Path traversal blocked",
			input:     "../../secrets.txt",
			want:      "",
			expectErr: true,
		},
		{
			name:      "Hidden traversal blocked",
			input:     "src/foo/../bar.go",
			want:      "",
			expectErr: true,
		},
		{
			name:      "Cleans redundant slashes",
			input:     "src//foo///bar.go",
			want:      "src/foo/bar.go",
			expectErr: false,
		},
		{
			name:      "Current directory",
			input:     ".",
			want:      ".",
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pathutil.Clean(tt.input)
			if (err != nil) != tt.expectErr {
				t.Errorf("Clean() error = %v, expectErr %v", err, tt.expectErr)
				return
			}
			if got != tt.want {
				t.Errorf("Clean() got = %v, want %v", got, tt.want)
			}
		})
	}
}