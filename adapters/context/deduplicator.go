package context

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

// SemanticDeduplicator tracks context chunks to prevent redundant LLM token usage across turns.
type SemanticDeduplicator struct {
	seenHashes map[string]bool
	mu         sync.RWMutex
}

func NewSemanticDeduplicator() *SemanticDeduplicator {
	return &SemanticDeduplicator{
		seenHashes: make(map[string]bool),
	}
}

// IsUnique checks if the exact content string has already been embedded in the context.
// If it is unique, it registers it and returns true.
func (d *SemanticDeduplicator) IsUnique(content string) bool {
	hash := d.hashContent(content)

	d.mu.RLock()
	if d.seenHashes[hash] {
		d.mu.RUnlock()
		return false
	}
	d.mu.RUnlock()

	d.mu.Lock()
	defer d.mu.Unlock()
	
	if d.seenHashes[hash] {
		return false
	}
	d.seenHashes[hash] = true
	return true
}

func (d *SemanticDeduplicator) hashContent(content string) string {
	h := sha256.New()
	h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(nil))
}

// Clear resets the deduplicator for a new agent session.
func (d *SemanticDeduplicator) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seenHashes = make(map[string]bool)
}