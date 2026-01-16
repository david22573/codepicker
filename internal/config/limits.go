package config

import (
	"os"
	"strconv"
	"time"
)

type Limits struct {
	MaxQueryLength     int
	MaxModelLength     int
	MaxBodySize        int64
	MaxFileSize        int64
	MaxCommandOutput   int
	CommandTimeout     time.Duration
	AgentMaxTurns      int
	AgentTimeout       time.Duration
	DailyCostLimit     float64
	RateLimitPerMinute float64
	RateLimitBurst     int
}

func DefaultLimits() *Limits {
	return &Limits{
		// HTTP / Server Limits
		MaxQueryLength: getEnvInt("MAX_QUERY_LENGTH", 25000),
		MaxModelLength: getEnvInt("MAX_MODEL_LENGTH", 100),
		MaxBodySize:    getEnvInt64("MAX_BODY_SIZE", 1024*1024), // 1MB

		// File System Limits
		MaxFileSize: getEnvInt64("MAX_FILE_SIZE", 100*1024*1024), // 100MB

		// Agent / Sandbox Limits
		MaxCommandOutput: getEnvInt("MAX_COMMAND_OUTPUT", 1024*100), // 100KB
		CommandTimeout:   getEnvDuration("COMMAND_TIMEOUT", 10*time.Second),
		AgentMaxTurns:    getEnvInt("AGENT_MAX_TURNS", 15),
		AgentTimeout:     getEnvDuration("AGENT_TIMEOUT", 5*time.Minute),

		// Cost & Rate Limits
		DailyCostLimit:     getEnvFloat("DAILY_COST_LIMIT", 10.0), // $10.00
		RateLimitPerMinute: getEnvFloat("RATE_LIMIT_PER_MIN", 10.0),
		RateLimitBurst:     getEnvInt("RATE_LIMIT_BURST", 10),
	}
}

// Helper functions to read env vars
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
