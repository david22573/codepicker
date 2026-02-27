package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
)

// PrefixCache memoizes the token cost of static conversation prefixes
// like the System Prompt and Tool Schemas.
type PrefixCache struct {
	estimator TokenEstimator
	cache     sync.Map
}

// NewPrefixCache creates a new cache utilizing the provided token estimator.
func NewPrefixCache(estimator TokenEstimator) *PrefixCache {
	return &PrefixCache{
		estimator: estimator,
	}
}

// PrefixSignature represents the unique state of the static context.
type PrefixSignature struct {
	SystemPrompt string
	Tools        []ToolDefinition
}

// GetEstimatedTokens computes or retrieves the token cost of the prefix.
func (p *PrefixCache) GetEstimatedTokens(sig PrefixSignature) int {
	hash := p.hashSignature(sig)

	if val, ok := p.cache.Load(hash); ok {
		return val.(int)
	}

	// 1. Estimate System Prompt as a mock message
	sysTokens := p.estimator.EstimateMessages([]Message{
		{Role: "system", Content: sig.SystemPrompt},
	})

	// 2. Estimate Tool Definitions (approximate overhead)
	toolTokens := 0
	if len(sig.Tools) > 0 {
		toolData, _ := json.Marshal(sig.Tools)
		toolTokens = p.estimator.EstimateText(string(toolData))
	}

	total := sysTokens + toolTokens

	p.cache.Store(hash, total)
	return total
}

func (p *PrefixCache) hashSignature(sig PrefixSignature) string {
	h := sha256.New()
	h.Write([]byte(sig.SystemPrompt))
	
	if len(sig.Tools) > 0 {
		toolData, _ := json.Marshal(sig.Tools)
		h.Write(toolData)
	}
	
	return hex.EncodeToString(h.Sum(nil))
}