package generation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
)

const ideaCacheSize = 32

type ideaCache struct {
	mu    sync.Mutex
	order []string
	m     map[string]string
}

func newIdeaCache() *ideaCache {
	return &ideaCache{m: map[string]string{}}
}

func ideaCacheKey(systemPrompt, userMessage, presetType, model string, maxTokens int) string {
	b, _ := json.Marshal([]any{systemPrompt, userMessage, presetType, model, maxTokens})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (c *ideaCache) get(key string) (string, bool) {
	if c == nil {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.m[key]
	return r, ok
}

func (c *ideaCache) put(key string, r string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.m[key]; !ok {
		c.order = append(c.order, key)
	}
	c.m[key] = r
	for len(c.order) > ideaCacheSize {
		delete(c.m, c.order[0])
		c.order = c.order[1:]
	}
}
