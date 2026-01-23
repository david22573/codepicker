package agent

import (
	"fmt"
	"regexp"
	"strings"
)

type RecoveryStrategy struct {
	Name        string
	Pattern     *regexp.Regexp
	Diagnosis   string
	FixCommands []CommandSequence
	MaxRetries  int
}

type CommandSequence struct {
	Binary string
	Args   []string
}

type RecoveryResult struct {
	Attempted    bool
	Success      bool
	StrategyUsed string
	ActionsTaken []string
	FinalOutput  string
	FinalError   error
}

var CommonFailures = []RecoveryStrategy{
	// ========== Go Language Errors ==========
	{
		Name:      "MissingGoMod",
		Pattern:   regexp.MustCompile(`go: go\.mod file not found`),
		Diagnosis: "Missing go.mod file - module not initialized",
		FixCommands: []CommandSequence{
			{Binary: "go", Args: []string{"mod", "init", "temp-module"}},
		},
		MaxRetries: 1,
	},
	{
		Name:      "MissingDependencies",
		Pattern:   regexp.MustCompile(`cannot find package|missing go\.sum entry|no required module provides`),
		Diagnosis: "Missing dependencies - running go mod tidy",
		FixCommands: []CommandSequence{
			{Binary: "go", Args: []string{"mod", "download"}},
			{Binary: "go", Args: []string{"mod", "tidy"}},
		},
		MaxRetries: 1,
	},
	{
		Name:      "BuildCacheProblem",
		Pattern:   regexp.MustCompile(`build cache is disabled|cache verification failed`),
		Diagnosis: "Build cache issue - cleaning and retrying",
		FixCommands: []CommandSequence{
			{Binary: "go", Args: []string{"clean", "-cache"}},
		},
		MaxRetries: 1,
	},

	// ========== Python Language Errors ==========
	{
		Name:      "PythonModuleMissing",
		Pattern:   regexp.MustCompile(`ModuleNotFoundError: No module named '(\w+)'|ImportError: No module named (\w+)`),
		Diagnosis: "Python module not installed",
		FixCommands: []CommandSequence{
			{Binary: "pip", Args: []string{"install", "$1"}}, // $1 will be replaced by captured group
		},
		MaxRetries: 1,
	},
	{
		Name:      "PythonPipMissing",
		Pattern:   regexp.MustCompile(`pip: command not found|'pip' is not recognized`),
		Diagnosis: "pip package manager not installed",
		FixCommands: []CommandSequence{
			{Binary: "python3", Args: []string{"-m", "ensurepip", "--upgrade"}},
		},
		MaxRetries: 1,
	},
	{
		Name:      "PythonVenvNotActivated",
		Pattern:   regexp.MustCompile(`cannot import name '.*' from 'venv'|No module named 'venv'`),
		Diagnosis: "Virtual environment not activated or not created",
		FixCommands: []CommandSequence{
			{Binary: "python3", Args: []string{"-m", "venv", "venv"}},
		},
		MaxRetries: 1,
	},
	{
		Name:        "PythonSyntaxError",
		Pattern:     regexp.MustCompile(`SyntaxError: invalid syntax|IndentationError:`),
		Diagnosis:   "Python syntax error detected - please fix code manually",
		FixCommands: []CommandSequence{
			// No auto-fix for syntax errors, just diagnose
		},
		MaxRetries: 0,
	},

	// ========== Node.js/npm Errors ==========
	{
		Name:      "NodeModuleMissing",
		Pattern:   regexp.MustCompile(`Cannot find module '([^']+)'|Error: Cannot resolve module '([^']+)'`),
		Diagnosis: "Node.js module not installed",
		FixCommands: []CommandSequence{
			{Binary: "npm", Args: []string{"install", "$1"}},
		},
		MaxRetries: 1,
	},
	{
		Name:        "NpmNotInstalled",
		Pattern:     regexp.MustCompile(`npm: command not found|'npm' is not recognized`),
		Diagnosis:   "npm package manager not installed",
		FixCommands: []CommandSequence{
			// Can't auto-fix this - requires Node.js installation
		},
		MaxRetries: 0,
	},
	{
		Name:      "PackageJsonMissing",
		Pattern:   regexp.MustCompile(`ENOENT: no such file or directory.*package\.json`),
		Diagnosis: "package.json not found - initializing npm project",
		FixCommands: []CommandSequence{
			{Binary: "npm", Args: []string{"init", "-y"}},
		},
		MaxRetries: 1,
	},
	{
		Name:      "NodeModulesCorrupted",
		Pattern:   regexp.MustCompile(`node_modules.*appears to be corrupted|EINTEGRITY|sha512.*integrity check failed`),
		Diagnosis: "node_modules corrupted - reinstalling dependencies",
		FixCommands: []CommandSequence{
			{Binary: "rm", Args: []string{"-rf", "node_modules"}},
			{Binary: "npm", Args: []string{"install"}},
		},
		MaxRetries: 1,
	},

	// ========== File System Permission Errors ==========
	{
		Name:      "PermissionDenied",
		Pattern:   regexp.MustCompile(`permission denied|EACCES|operation not permitted`),
		Diagnosis: "Permission denied - attempting to fix file permissions",
		FixCommands: []CommandSequence{
			{Binary: "chmod", Args: []string{"+x", "$FILE"}}, // $FILE should be extracted from error context
		},
		MaxRetries: 1,
	},
	{
		Name:      "ScriptNotExecutable",
		Pattern:   regexp.MustCompile(`.*: Permission denied|bash: \./(.*): Permission denied`),
		Diagnosis: "Script file not executable - making it executable",
		FixCommands: []CommandSequence{
			{Binary: "chmod", Args: []string{"+x", "$1"}},
		},
		MaxRetries: 1,
	},

	// ========== Git Errors ==========
	{
		Name:      "GitMergeConflict",
		Pattern:   regexp.MustCompile(`CONFLICT \(content\): Merge conflict|Automatic merge failed; fix conflicts`),
		Diagnosis: "Git merge conflict detected - manual resolution required",
		FixCommands: []CommandSequence{
			{Binary: "git", Args: []string{"status"}},
			// No auto-resolution, requires human intervention
		},
		MaxRetries: 0,
	},
	{
		Name:      "GitNotInitialized",
		Pattern:   regexp.MustCompile(`fatal: not a git repository|not a git repository \(or any of the parent directories\)`),
		Diagnosis: "Git repository not initialized",
		FixCommands: []CommandSequence{
			{Binary: "git", Args: []string{"init"}},
		},
		MaxRetries: 1,
	},
	{
		Name:      "GitUnstagedChanges",
		Pattern:   regexp.MustCompile(`error: Your local changes to the following files would be overwritten|Please commit your changes or stash them`),
		Diagnosis: "Unstaged changes blocking git operation",
		FixCommands: []CommandSequence{
			{Binary: "git", Args: []string{"stash"}},
		},
		MaxRetries: 1,
	},

	// ========== Docker Errors ==========
	{
		Name:        "DockerDaemonNotRunning",
		Pattern:     regexp.MustCompile(`Cannot connect to the Docker daemon|Is the docker daemon running\?`),
		Diagnosis:   "Docker daemon is not running",
		FixCommands: []CommandSequence{
			// Can't auto-start daemon from unprivileged context
		},
		MaxRetries: 0,
	},
	{
		Name:      "DockerImageNotFound",
		Pattern:   regexp.MustCompile(`Unable to find image '([^']+)' locally|Error response from daemon: pull access denied for ([^,]+)`),
		Diagnosis: "Docker image not available locally - pulling image",
		FixCommands: []CommandSequence{
			{Binary: "docker", Args: []string{"pull", "$1"}},
		},
		MaxRetries: 1,
	},

	// ========== Network/Connectivity Errors ==========
	{
		Name:        "NetworkTimeout",
		Pattern:     regexp.MustCompile(`dial tcp.*i/o timeout|net/http: request canceled|context deadline exceeded`),
		Diagnosis:   "Network timeout - retrying operation",
		FixCommands: []CommandSequence{
			// Will be retried automatically by the recovery loop
		},
		MaxRetries: 2,
	},
	{
		Name:        "DNSResolutionFailed",
		Pattern:     regexp.MustCompile(`no such host|could not resolve host|Temporary failure in name resolution`),
		Diagnosis:   "DNS resolution failed - check network connectivity",
		FixCommands: []CommandSequence{
			// No auto-fix for DNS issues
		},
		MaxRetries: 0,
	},

	// ========== Database Errors ==========
	{
		Name:        "DatabaseLocked",
		Pattern:     regexp.MustCompile(`database is locked|SQLITE_BUSY`),
		Diagnosis:   "Database locked by another process - retrying",
		FixCommands: []CommandSequence{
			// Will be retried automatically
		},
		MaxRetries: 3,
	},

	// ========== Make/Build Tool Errors ==========
	{
		Name:        "MakefileNotFound",
		Pattern:     regexp.MustCompile(`make: \*\*\* No targets specified and no makefile found`),
		Diagnosis:   "Makefile not found in current directory",
		FixCommands: []CommandSequence{
			// No auto-fix
		},
		MaxRetries: 0,
	},
	{
		Name:        "MissingBuildTool",
		Pattern:     regexp.MustCompile(`(gcc|g\+\+|clang|cargo|mvn|gradle): command not found`),
		Diagnosis:   "Build tool not installed: $1",
		FixCommands: []CommandSequence{
			// No auto-install for compilers/build tools
		},
		MaxRetries: 0,
	},
}

// ExecuteWithRecovery wraps the Sentinel execution with auto-recovery logic
func (e *Engine) ExecuteWithRecovery(binary string, args []string, maxAttempts int) RecoveryResult {
	result := RecoveryResult{
		Attempted: false,
		Success:   false,
	}

	// 1. Initial execution attempt
	output, err := e.Sentinel.Execute(binary, args)
	result.FinalOutput = output
	result.FinalError = err

	if err == nil {
		result.Success = true
		return result
	}

	// 2. Attempt recovery
	for _, strategy := range CommonFailures {
		if !strategy.Pattern.MatchString(output) && !strategy.Pattern.MatchString(err.Error()) {
			continue
		}

		// Skip strategies with no fix commands (diagnostic only)
		if len(strategy.FixCommands) == 0 {
			e.Logger.Info(fmt.Sprintf("ℹ️  Diagnosis: %s", strategy.Diagnosis))
			e.Logger.Info("💡 This error requires manual intervention")
			result.Attempted = true
			result.StrategyUsed = strategy.Name
			return result
		}

		result.Attempted = true
		result.StrategyUsed = strategy.Name

		e.Logger.Info(fmt.Sprintf("🚑 Auto-recovery triggered: %s", strategy.Diagnosis))

		// Extract captured groups for parameter substitution
		matches := strategy.Pattern.FindStringSubmatch(output)
		if len(matches) == 0 {
			matches = strategy.Pattern.FindStringSubmatch(err.Error())
		}

		// Execute fix commands
		for _, fixCmd := range strategy.FixCommands {
			// Substitute captured groups in arguments
			resolvedArgs := make([]string, len(fixCmd.Args))
			for i, arg := range fixCmd.Args {
				resolvedArgs[i] = substituteCaptures(arg, matches)
			}

			cmdStr := fmt.Sprintf("%s %s", fixCmd.Binary, strings.Join(resolvedArgs, " "))
			result.ActionsTaken = append(result.ActionsTaken, cmdStr)

			e.Logger.Debug(fmt.Sprintf("Running fix: %s", cmdStr))
			fixOutput, fixErr := e.Sentinel.Execute(fixCmd.Binary, resolvedArgs)

			if fixErr != nil {
				e.Logger.Warn(fmt.Sprintf("Recovery step failed: %v", fixErr))
				e.Logger.Debug(fmt.Sprintf("Fix Output: %s", fixOutput))
				// We continue anyway, as subsequent fix steps might help
			}
		}

		// Retry original command based on strategy's MaxRetries
		for retry := 0; retry < strategy.MaxRetries; retry++ {
			e.Logger.Debug(fmt.Sprintf("Retrying original command (attempt %d/%d)...", retry+1, strategy.MaxRetries))
			output, err = e.Sentinel.Execute(binary, args)
			result.FinalOutput = output
			result.FinalError = err

			if err == nil {
				result.Success = true
				e.Logger.Info(fmt.Sprintf("✅ Recovery successful using strategy: %s", strategy.Name))
				return result
			}

			// Check if the same error still exists
			if !strategy.Pattern.MatchString(output) && !strategy.Pattern.MatchString(err.Error()) {
				// Different error now - might be progress
				e.Logger.Debug("Error signature changed, may be making progress")
				break
			}
		}

		if result.FinalError != nil {
			e.Logger.Warn("❌ Recovery attempted but original command still failed.")
			e.Logger.Debug(fmt.Sprintf("Final error: %v", result.FinalError))
		}

		// We break after the first matching strategy to avoid cascading specialized fixes
		break
	}

	return result
}

// substituteCaptures replaces $1, $2, etc. in a string with captured groups from regex
func substituteCaptures(template string, captures []string) string {
	result := template
	for i := 1; i < len(captures); i++ {
		placeholder := fmt.Sprintf("$%d", i)
		result = strings.ReplaceAll(result, placeholder, captures[i])
	}
	// Handle special $FILE placeholder by using first capture if available
	if strings.Contains(result, "$FILE") && len(captures) > 1 {
		result = strings.ReplaceAll(result, "$FILE", captures[1])
	}
	return result
}

// GetRecoveryStrategies returns all available recovery strategies
// Useful for documentation or debugging
func GetRecoveryStrategies() []RecoveryStrategy {
	return CommonFailures
}

// FindStrategy finds a recovery strategy by name
func FindStrategy(name string) *RecoveryStrategy {
	for _, strategy := range CommonFailures {
		if strategy.Name == name {
			return &strategy
		}
	}
	return nil
}

// TestPattern allows testing if an error matches any recovery pattern
// Useful for debugging and testing
func TestPattern(errorText string) []string {
	var matches []string
	for _, strategy := range CommonFailures {
		if strategy.Pattern.MatchString(errorText) {
			matches = append(matches, strategy.Name)
		}
	}
	return matches
}
