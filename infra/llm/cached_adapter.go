package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	domainContext "github.com/david22573/codepicker/domain/context"
)

// CachedAdapter wraps an LLM Provider to memoize responses.
// This is highly effective in CI/CD pipelines or local iterative debugging.
type CachedAdapter struct {
	underlying Provider
	cacheDir   string
	enabled    bool
	mu         sync.RWMutex
}

// NewCachedAdapter initializes the caching layer. If enabled, it ensures the cache directory exists.
func NewCachedAdapter(underlying Provider, cacheDir string, enabled bool) *CachedAdapter {
	if enabled {
		_ = os.MkdirAll(cacheDir, 0755)
	}
	return &CachedAdapter{
		underlying: underlying,
		cacheDir:   cacheDir,
		enabled:    enabled,
	}
}

type cachedPayload struct {
	Message Message                  `json:"message"`
	Usage   domainContext.TokenUsage `json:"usage"`
}

func (c *CachedAdapter) hashRequest(messages []Message, tools []ToolDefinition) string {
	h := sha256.New()
	msgBytes, _ := json.Marshal(messages)
	h.Write(msgBytes)

	if len(tools) > 0 {
		toolBytes, _ := json.Marshal(tools)
		h.Write(toolBytes)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ChatNative intercepts the structured chat request, serving from cache if available.
func (c *CachedAdapter) ChatNative(ctx context.Context, messages []Message, tools []ToolDefinition) (Message, domainContext.TokenUsage, error) {
	if !c.enabled {
		return c.underlying.ChatNative(ctx, messages, tools)
	}

	hash := c.hashRequest(messages, tools)
	cachePath := filepath.Join(c.cacheDir, hash+".json")

	// 1. Check Cache
	c.mu.RLock()
	data, err := os.ReadFile(cachePath)
	c.mu.RUnlock()

	if err == nil {
		var payload cachedPayload
		if err := json.Unmarshal(data, &payload); err == nil {
			fmt.Println("⚡ [LLM CACHE] Hit: Returning memoized response.")
			return payload.Message, payload.Usage, nil
		}
	}

	// 2. Cache Miss: Call Underlying Provider
	msg, usage, err := c.underlying.ChatNative(ctx, messages, tools)

	// 3. Store Result
	if err == nil {
		payload := cachedPayload{Message: msg, Usage: usage}
		if data, err := json.MarshalIndent(payload, "", "  "); err == nil {
			c.mu.Lock()
			_ = os.WriteFile(cachePath, data, 0644)
			c.mu.Unlock()
		}
	}

	return msg, usage, err
}

// Chat maintains compatibility with the simpler text-only interface.
func (c *CachedAdapter) Chat(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	messages := []Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}
	msg, _, err := c.ChatNative(ctx, messages, nil)
	return msg.Content, err
}
