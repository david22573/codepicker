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
		if !strategy.Pattern.MatchString(output) {
			continue
		}

		result.Attempted = true
		result.StrategyUsed = strategy.Name

		e.Logger.Info(fmt.Sprintf("🚑 Auto-recovery triggered: %s", strategy.Diagnosis))

		// Execute fix commands
		for _, fixCmd := range strategy.FixCommands {
			cmdStr := fmt.Sprintf("%s %s", fixCmd.Binary, strings.Join(fixCmd.Args, " "))
			result.ActionsTaken = append(result.ActionsTaken, cmdStr)

			e.Logger.Debug(fmt.Sprintf("Running fix: %s", cmdStr))
			fixOutput, fixErr := e.Sentinel.Execute(fixCmd.Binary, fixCmd.Args)

			if fixErr != nil {
				e.Logger.Warn(fmt.Sprintf("Recovery step failed: %v", fixErr))
				e.Logger.Debug(fmt.Sprintf("Fix Output: %s", fixOutput))
				// We continue anyway, as subsequent fix steps might help
			}
		}

		// Retry original command
		e.Logger.Debug("Retrying original command...")
		output, err = e.Sentinel.Execute(binary, args)
		result.FinalOutput = output
		result.FinalError = err

		if err == nil {
			result.Success = true
			e.Logger.Info(fmt.Sprintf("✅ Recovery successful using strategy: %s", strategy.Name))
			return result
		} else {
			e.Logger.Warn("❌ Recovery attempted but original command still failed.")
		}

		// We break after the first matching strategy to avoid cascading specialized fixes
		break
	}

	return result
}
