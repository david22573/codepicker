package paths_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/david22573/codepicker/internal/paths"
)

func TestSanitize(t *testing.T) {
	// Setup a temp directory for valid path testing
	tmpDir, err := os.MkdirTemp("", "codepicker_paths_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Change cwd to temp dir to simulate running inside a project
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Create a dummy file
	if err := os.WriteFile("valid_file.txt", []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		input   string
		wantErr bool
		errCode string // Optional substring check
	}{
		{
			name:    "Valid relative file",
			input:   "valid_file.txt",
			wantErr: false,
		},
		{
			name:    "Valid current directory",
			input:   ".",
			wantErr: false,
		},
		{
			name:    "Empty path",
			input:   "",
			wantErr: true,
			errCode: "VALIDATION_ERROR",
		},
		{
			name:    "Parent directory traversal",
			input:   "../outside",
			wantErr: true,
			errCode: "path traversal",
		},
		{
			name:    "Absolute system path (passwd)",
			input:   "/etc/passwd",
			wantErr: true,
			errCode: "system directory",
		},
		{
			name:    "Absolute system path (proc)",
			input:   "/proc/self/status",
			wantErr: true,
			errCode: "system directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := paths.Sanitize(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("Sanitize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil {
				if !strings.Contains(err.Error(), tt.errCode) {
					t.Errorf("expected error containing %q, got %v", tt.errCode, err)
				}
			} else if !tt.wantErr {
				// For success, ensure path is absolute and clean
				if !filepath.IsAbs(got) {
					t.Errorf("expected absolute path, got %s", got)
				}
			}
		})
	}
}

func TestValidateOutput(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"Standard output", "codepicker_out.md", false},
		{"Subdirectory output", "docs/context.md", false},
		{"Critical file overwrite (go.mod)", "go.mod", true},
		{"Critical file overwrite (.git)", ".git", true},
		{"Dotfile protection", ".env", true},
		{"Allowed md dotfile", ".hidden.md", false},
	}

	// We need to create dummy files for Sanitize to resolve them successfully
	// or mock the FS. Since Sanitize checks EvalSymlinks/Abs, strictly speaking
	// the file usually needs to be "resolvable" relative to CWD.
	// For this unit test, we rely on ValidateOutput's logic which calls Sanitize.
	// We'll skip creating physical files and assume Sanitize handles non-existent paths gracefully
	// (which it does via EvalSymlinks logic fallback in your implementation).

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := paths.ValidateOutput(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateOutput(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}
