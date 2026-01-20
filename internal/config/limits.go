package config

import (
	"os"
	"strconv"
	"time"
)

type Limits struct {
	MaxQueryLength int
	MaxModelLength int
	MaxBodySize    int64
	MaxFileSize    int64

	// [4.2] Output Ceilings
	MaxCommandOutput int // Bytes
	MaxToolOutput    int // Bytes (New: general limit for any tool)
	MaxStepTokens    int // New: Max tokens the AI can generate per turn

	CommandTimeout time.Duration
	AgentMaxTurns  int
	AgentTimeout   time.Duration

	// [4.1] Cost Controls
	DailyCostLimit     float64
	RateLimitPerMinute float64
	RateLimitBurst     int

	MaxRecoveryAttempts int
}

func DefaultLimits() *Limits {
	return &Limits{
		MaxQueryLength: getEnvInt("MAX_QUERY_LENGTH", 25000),
		MaxModelLength: getEnvInt("MAX_MODEL_LENGTH", 100),
		MaxBodySize:    getEnvInt64("MAX_BODY_SIZE", 1024*1024),
		MaxFileSize:    getEnvInt64("MAX_FILE_SIZE", 100*1024*1024),

		// [4.2] Default Limits
		MaxCommandOutput: getEnvInt("MAX_COMMAND_OUTPUT", 1024*50), // 50KB shell output
		MaxToolOutput:    getEnvInt("MAX_TOOL_OUTPUT", 1024*100),   // 100KB general tool output
		MaxStepTokens:    getEnvInt("MAX_STEP_TOKENS", 2000),       // Prevent runaway generation

		CommandTimeout: getEnvDuration("COMMAND_TIMEOUT", 2*time.Minute), // Increased default
		AgentMaxTurns:  getEnvInt("AGENT_MAX_TURNS", 30),
		AgentTimeout:   getEnvDuration("AGENT_TIMEOUT", 15*time.Minute),

		DailyCostLimit:     getEnvFloat("DAILY_COST_LIMIT", 5.0), // Lower default for safety
		RateLimitPerMinute: getEnvFloat("RATE_LIMIT_PER_MIN", 10.0),
		RateLimitBurst:     getEnvInt("RATE_LIMIT_BURST", 10),

		MaxRecoveryAttempts: getEnvInt("MAX_RECOVERY_ATTEMPTS", 2),
	}
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvInt64(key string, defaultVal int64) int64 {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.ParseInt(val, 10, 64); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvFloat(key string, defaultVal float64) float64 {
	if val := os.Getenv(key); val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return defaultVal
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}
