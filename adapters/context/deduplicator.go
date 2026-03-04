package context

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
)

type SemanticDeduplicator struct {
	seenHashes map[string]bool
	mu         sync.RWMutex
}

func NewSemanticDeduplicator() *SemanticDeduplicator {
	return &SemanticDeduplicator{
		seenHashes: make(map[string]bool),
	}
}

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
	// Strip whitespace and normalize formatting before hashing to increase deduplication rates
	normalized := strings.Join(strings.Fields(content), " ")
	h := sha256.New()
	h.Write([]byte(normalized))
	return hex.EncodeToString(h.Sum(nil))
}

func (d *SemanticDeduplicator) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seenHashes = make(map[string]bool)
}
