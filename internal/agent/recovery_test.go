package agent

import (
	"testing"
)

func TestRecoveryPatternMatching(t *testing.T) {
	tests := []struct {
		name          string
		errorOutput   string
		expectedMatch string
	}{
		// Go Errors
		{
			name:          "Go missing go.mod",
			errorOutput:   "go: go.mod file not found in current directory or any parent directory",
			expectedMatch: "MissingGoMod",
		},
		{
			name:          "Go missing dependencies",
			errorOutput:   "cannot find package \"github.com/pkg/errors\" in any of:",
			expectedMatch: "MissingDependencies",
		},
		{
			name:          "Go build cache issue",
			errorOutput:   "build cache is disabled by GOCACHE=off",
			expectedMatch: "BuildCacheProblem",
		},

		// Python Errors
		{
			name:          "Python module not found",
			errorOutput:   "ModuleNotFoundError: No module named 'requests'",
			expectedMatch: "PythonModuleMissing",
		},
		{
			name:          "Python import error legacy format",
			errorOutput:   "ImportError: No module named requests",
			expectedMatch: "PythonModuleMissing",
		},
		{
			name:          "Python pip missing",
			errorOutput:   "bash: pip: command not found",
			expectedMatch: "PythonPipMissing",
		},
		{
			name:          "Python syntax error",
			errorOutput:   "SyntaxError: invalid syntax at line 42",
			expectedMatch: "PythonSyntaxError",
		},
		{
			name:          "Python indentation error",
			errorOutput:   "IndentationError: unexpected indent",
			expectedMatch: "PythonSyntaxError",
		},

		// Node.js/npm Errors
		{
			name:          "Node module not found single quotes",
			errorOutput:   "Error: Cannot find module 'express'",
			expectedMatch: "NodeModuleMissing",
		},
		{
			name:          "Node module not found resolve error",
			errorOutput:   "Error: Cannot resolve module 'lodash'",
			expectedMatch: "NodeModuleMissing",
		},
		{
			name:          "npm not installed",
			errorOutput:   "bash: npm: command not found",
			expectedMatch: "NpmNotInstalled",
		},
		{
			name:          "package.json missing",
			errorOutput:   "ENOENT: no such file or directory, open '/path/to/package.json'",
			expectedMatch: "PackageJsonMissing",
		},
		{
			name:          "node_modules corrupted integrity check",
			errorOutput:   "npm ERR! sha512-abc123 integrity check failed when using sha512",
			expectedMatch: "NodeModulesCorrupted",
		},
		{
			name:          "node_modules appears corrupted",
			errorOutput:   "The node_modules directory appears to be corrupted",
			expectedMatch: "NodeModulesCorrupted",
		},

		// Permission Errors
		{
			name:          "Permission denied generic",
			errorOutput:   "bash: ./script.sh: Permission denied",
			expectedMatch: "ScriptNotExecutable",
		},
		{
			name:          "Permission denied EACCES",
			errorOutput:   "Error: EACCES: permission denied, open '/etc/file'",
			expectedMatch: "PermissionDenied",
		},
		{
			name:          "Operation not permitted",
			errorOutput:   "operation not permitted: cannot write to /sys/kernel",
			expectedMatch: "PermissionDenied",
		},

		// Git Errors
		{
			name:          "Git merge conflict",
			errorOutput:   "CONFLICT (content): Merge conflict in README.md",
			expectedMatch: "GitMergeConflict",
		},
		{
			name:          "Git automatic merge failed",
			errorOutput:   "Automatic merge failed; fix conflicts and then commit the result.",
			expectedMatch: "GitMergeConflict",
		},
		{
			name:          "Git not initialized",
			errorOutput:   "fatal: not a git repository (or any of the parent directories): .git",
			expectedMatch: "GitNotInitialized",
		},
		{
			name:          "Git unstaged changes",
			errorOutput:   "error: Your local changes to the following files would be overwritten by merge:",
			expectedMatch: "GitUnstagedChanges",
		},

		// Docker Errors
		{
			name:          "Docker daemon not running",
			errorOutput:   "Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?",
			expectedMatch: "DockerDaemonNotRunning",
		},
		{
			name:          "Docker image not found",
			errorOutput:   "Unable to find image 'nginx:latest' locally",
			expectedMatch: "DockerImageNotFound",
		},

		// Network Errors
		{
			name:          "Network timeout i/o",
			errorOutput:   "dial tcp 192.168.1.1:443: i/o timeout",
			expectedMatch: "NetworkTimeout",
		},
		{
			name:          "Context deadline exceeded",
			errorOutput:   "Get \"https://api.example.com\": context deadline exceeded",
			expectedMatch: "NetworkTimeout",
		},
		{
			name:          "DNS resolution failed",
			errorOutput:   "dial tcp: lookup api.example.com: no such host",
			expectedMatch: "DNSResolutionFailed",
		},

		// Database Errors
		{
			name:          "SQLite database locked",
			errorOutput:   "database is locked",
			expectedMatch: "DatabaseLocked",
		},

		// Build Tool Errors
		{
			name:          "Makefile not found",
			errorOutput:   "make: *** No targets specified and no makefile found.  Stop.",
			expectedMatch: "MakefileNotFound",
		},
		{
			name:          "gcc not found",
			errorOutput:   "bash: gcc: command not found",
			expectedMatch: "MissingBuildTool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := TestPattern(tt.errorOutput)

			if len(matches) == 0 {
				t.Errorf("Expected to match strategy %s, but got no matches", tt.expectedMatch)
				return
			}

			found := false
			for _, match := range matches {
				if match == tt.expectedMatch {
					found = true
					break
				}
			}

			if !found {
				t.Errorf("Expected to match strategy %s, but got %v", tt.expectedMatch, matches)
			}
		})
	}
}

func TestSubstituteCaptures(t *testing.T) {
	tests := []struct {
		name     string
		template string
		captures []string
		expected string
	}{
		{
			name:     "No substitution needed",
			template: "install package",
			captures: []string{"full match"},
			expected: "install package",
		},
		{
			name:     "Single capture group",
			template: "pip install $1",
			captures: []string{"full match", "requests"},
			expected: "pip install requests",
		},
		{
			name:     "Multiple capture groups",
			template: "npm install $1 --save-dev $2",
			captures: []string{"full match", "lodash", "webpack"},
			expected: "npm install lodash --save-dev webpack",
		},
		{
			name:     "FILE placeholder with capture",
			template: "chmod +x $FILE",
			captures: []string{"full match", "script.sh"},
			expected: "chmod +x script.sh",
		},
		{
			name:     "No captures provided",
			template: "pip install $1",
			captures: []string{},
			expected: "pip install $1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := substituteCaptures(tt.template, tt.captures)
			if result != tt.expected {
				t.Errorf("substituteCaptures() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestFindStrategy(t *testing.T) {
	tests := []struct {
		name         string
		strategyName string
		shouldFind   bool
	}{
		{
			name:         "Find existing Go strategy",
			strategyName: "MissingGoMod",
			shouldFind:   true,
		},
		{
			name:         "Find Python strategy",
			strategyName: "PythonModuleMissing",
			shouldFind:   true,
		},
		{
			name:         "Find npm strategy",
			strategyName: "NodeModuleMissing",
			shouldFind:   true,
		},
		{
			name:         "Non-existent strategy",
			strategyName: "NonExistentStrategy",
			shouldFind:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy := FindStrategy(tt.strategyName)

			if tt.shouldFind && strategy == nil {
				t.Errorf("Expected to find strategy %s, but got nil", tt.strategyName)
			}

			if !tt.shouldFind && strategy != nil {
				t.Errorf("Expected not to find strategy %s, but got %+v", tt.strategyName, strategy)
			}

			if strategy != nil && strategy.Name != tt.strategyName {
				t.Errorf("Strategy name mismatch: got %s, want %s", strategy.Name, tt.strategyName)
			}
		})
	}
}

func TestGetRecoveryStrategies(t *testing.T) {
	strategies := GetRecoveryStrategies()

	if len(strategies) == 0 {
		t.Error("Expected at least one recovery strategy, got none")
	}

	// Verify we have strategies for major categories
	categories := map[string]bool{
		"Go":         false,
		"Python":     false,
		"Node":       false,
		"Permission": false,
		"Git":        false,
		"Docker":     false,
		"Network":    false,
	}

	for _, strategy := range strategies {
		if strategy.Name == "MissingGoMod" {
			categories["Go"] = true
		}
		if strategy.Name == "PythonModuleMissing" {
			categories["Python"] = true
		}
		if strategy.Name == "NodeModuleMissing" {
			categories["Node"] = true
		}
		if strategy.Name == "PermissionDenied" {
			categories["Permission"] = true
		}
		if strategy.Name == "GitMergeConflict" {
			categories["Git"] = true
		}
		if strategy.Name == "DockerDaemonNotRunning" {
			categories["Docker"] = true
		}
		if strategy.Name == "NetworkTimeout" {
			categories["Network"] = true
		}
	}

	for category, found := range categories {
		if !found {
			t.Errorf("No recovery strategy found for category: %s", category)
		}
	}
}

func TestRecoveryStrategyValidation(t *testing.T) {
	// Verify all strategies have required fields
	strategies := GetRecoveryStrategies()

	for _, strategy := range strategies {
		t.Run(strategy.Name, func(t *testing.T) {
			if strategy.Name == "" {
				t.Error("Strategy has empty name")
			}

			if strategy.Pattern == nil {
				t.Error("Strategy has nil pattern")
			}

			if strategy.Diagnosis == "" {
				t.Error("Strategy has empty diagnosis")
			}

			// Test that pattern can be used
			testStr := "test error message"
			_ = strategy.Pattern.MatchString(testStr)
		})
	}
}

func TestMultiplePatternMatches(t *testing.T) {
	// Some errors might match multiple patterns - verify we handle this gracefully
	errorText := "permission denied: cannot access go.mod file"

	matches := TestPattern(errorText)

	// This should match both PermissionDenied patterns
	if len(matches) == 0 {
		t.Error("Expected at least one match for multi-pattern error")
	}

	t.Logf("Matched strategies: %v", matches)
}

func BenchmarkTestPattern(b *testing.B) {
	errorText := "ModuleNotFoundError: No module named 'requests'"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = TestPattern(errorText)
	}
}

func BenchmarkSubstituteCaptures(b *testing.B) {
	template := "pip install $1"
	captures := []string{"full match", "package_name"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = substituteCaptures(template, captures)
	}
}
