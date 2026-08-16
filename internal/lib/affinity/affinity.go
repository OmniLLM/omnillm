// Package affinity implements channel (provider-instance) affinity for OmniLLM.
//
// Goal: keep a linear conversation pinned to the same upstream provider instance
// across turns so that upstream prompt-cache (Anthropic / OpenAI cached_tokens)
// stays warm, cutting input-token cost on multi-turn agent workloads.
//
// The affinity key is derived from the conversation *prefix* (system prompt +
// all messages except the final one). Requests sharing a prefix belong to the
// same cache chain and should therefore prefer the same instance.
//
// Affinity only re-orders the candidate list — it never locks. If the pinned
// instance is gone or fails, dispatch falls through to normal priority order,
// so fallback correctness is unchanged.
package affinity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"omnillm/internal/cif"
)

// Config controls affinity behaviour. Zero value is unusable; use DefaultConfig.
type Config struct {
	Enabled       bool
	TTL           time.Duration
	MaxEntries    int
	IncludeUserID bool
}

// DefaultConfig returns production defaults (enabled, 5m TTL, 50k entries).
func DefaultConfig() Config {
	return Config{
		Enabled:       true,
		TTL:           5 * time.Minute,
		MaxEntries:    50_000,
		IncludeUserID: true,
	}
}

type entry struct {
	instanceID string
	expiresAt  time.Time
}

// Cache is a bounded, TTL'd map from conversation-prefix hash to instance ID.
// Eviction is size-bounded (oldest-write pruned) plus lazy TTL on read.
type Cache struct {
	mu   sync.Mutex
	cfg  Config
	data map[string]entry
	// insertion order ring for cheap size-bounded eviction
	order []string

	hits   uint64
	misses uint64
}

// NewCache builds a Cache from cfg.
func NewCache(cfg Config) *Cache {
	if cfg.MaxEntries <= 0 {
		cfg.MaxEntries = 50_000
	}
	if cfg.TTL <= 0 {
		cfg.TTL = 5 * time.Minute
	}
	return &Cache{
		cfg:   cfg,
		data:  make(map[string]entry, 1024),
		order: make([]string, 0, 1024),
	}
}

// Enabled reports whether affinity is active.
func (c *Cache) Enabled() bool { return c != nil && c.cfg.Enabled }

// Stats returns hit/miss counters and current size.
func (c *Cache) Stats() (hits, misses uint64, size int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hits, c.misses, len(c.data)
}

// key builds the affinity cache key for a request. It hashes the STABLE head of
// the conversation: system prompt + the first message. That head is identical
// across every turn of a linear conversation, and it is precisely the prefix
// upstream prompt-cache keys on — so keying on it pins a conversation to one
// instance for its whole lifetime. Returns ("", false) when there's nothing to
// key on yet.
//
// Collision semantics are intentionally benign: two distinct conversations that
// share an identical head hash to the same key and thus prefer the same
// instance — which is exactly what we want, since they share the cacheable
// prefix. Affinity only re-orders candidates, so a collision never harms
// correctness.
func promptCachePrefix(request *cif.CanonicalRequest, requestedModel string, includeUserID bool) ([]byte, bool) {
	if request.PromptCache == nil && !requestHasBreakpoint(request) {
		return nil, false
	}
	prefix := struct {
		Model       string                     `json:"model"`
		UserID      *string                    `json:"userId,omitempty"`
		Tools       []cif.CIFTool              `json:"tools,omitempty"`
		System      []cif.CIFSystemBlock       `json:"system,omitempty"`
		Messages    []cif.CIFMessage           `json:"messages,omitempty"`
		PromptCache *cif.CIFPromptCacheRequest `json:"promptCache,omitempty"`
	}{Model: requestedModel, Tools: request.Tools, System: request.System, PromptCache: request.PromptCache}
	if includeUserID {
		prefix.UserID = request.UserID
	}
	lastMessage := -1
	if request.PromptCache != nil && request.PromptCache.Automatic != nil {
		lastMessage = len(request.Messages) - 1
	}
	for index, message := range request.Messages {
		if messageHasBreakpoint(message) {
			lastMessage = index
		}
	}
	if lastMessage >= 0 {
		messages := append([]cif.CIFMessage(nil), request.Messages[:lastMessage+1]...)
		messages[lastMessage] = truncateMessageAtBreakpoint(messages[lastMessage])
		prefix.Messages = messages
	}
	encoded, err := json.Marshal(prefix)
	return encoded, err == nil
}

func truncateMessageAtBreakpoint(message cif.CIFMessage) cif.CIFMessage {
	switch typed := message.(type) {
	case cif.CIFSystemMessage:
		last := lastSystemBreakpoint(typed.Content)
		if last >= 0 {
			typed.Content = append([]cif.CIFSystemBlock(nil), typed.Content[:last+1]...)
		}
		return typed
	case cif.CIFUserMessage:
		last := lastContentBreakpoint(typed.Content)
		if last >= 0 {
			typed.Content = append([]cif.CIFContentPart(nil), typed.Content[:last+1]...)
		}
		return typed
	case cif.CIFAssistantMessage:
		last := lastContentBreakpoint(typed.Content)
		if last >= 0 {
			typed.Content = append([]cif.CIFContentPart(nil), typed.Content[:last+1]...)
		}
		return typed
	default:
		return message
	}
}

func lastSystemBreakpoint(blocks []cif.CIFSystemBlock) int {
	last := -1
	for index, block := range blocks {
		if block.CacheControl != nil {
			last = index
		}
	}
	return last
}

func lastContentBreakpoint(parts []cif.CIFContentPart) int {
	last := -1
	for index, part := range parts {
		if partsHaveBreakpoint([]cif.CIFContentPart{part}) {
			last = index
		}
	}
	return last
}

func requestHasBreakpoint(request *cif.CanonicalRequest) bool {
	for _, tool := range request.Tools {
		if tool.CacheControl != nil {
			return true
		}
	}
	for _, block := range request.System {
		if block.CacheControl != nil {
			return true
		}
	}
	for _, message := range request.Messages {
		if messageHasBreakpoint(message) {
			return true
		}
	}
	return false
}

func messageHasBreakpoint(message cif.CIFMessage) bool {
	switch typed := message.(type) {
	case cif.CIFSystemMessage:
		for _, block := range typed.Content {
			if block.CacheControl != nil {
				return true
			}
		}
	case cif.CIFUserMessage:
		return partsHaveBreakpoint(typed.Content)
	case cif.CIFAssistantMessage:
		return partsHaveBreakpoint(typed.Content)
	}
	return false
}

func partsHaveBreakpoint(parts []cif.CIFContentPart) bool {
	for _, part := range parts {
		switch typed := part.(type) {
		case cif.CIFTextPart:
			if typed.CacheControl != nil {
				return true
			}
		case cif.CIFImagePart:
			if typed.CacheControl != nil {
				return true
			}
		case cif.CIFThinkingPart:
			if typed.CacheControl != nil {
				return true
			}
		case cif.CIFToolCallPart:
			if typed.CacheControl != nil {
				return true
			}
		case cif.CIFToolResultPart:
			if typed.CacheControl != nil {
				return true
			}
		}
	}
	return false
}

func (c *Cache) key(request *cif.CanonicalRequest, requestedModel string) (string, bool) {
	if request == nil {
		return "", false
	}
	if prefix, ok := promptCachePrefix(request, requestedModel, c.cfg.IncludeUserID); ok {
		h := sha256.Sum256(prefix)
		return "affinity:v2:" + hex.EncodeToString(h[:])[:32], true
	}
	if len(request.Messages) == 0 {
		return "", false
	}
	hasSystem := cif.PlainSystemText(request.System) != ""

	h := sha256.New()
	h.Write([]byte("m:"))
	h.Write([]byte(requestedModel))
	if c.cfg.IncludeUserID && request.UserID != nil {
		h.Write([]byte("|u:"))
		h.Write([]byte(*request.UserID))
	}
	if hasSystem {
		h.Write([]byte("|s:"))
		h.Write([]byte(cif.PlainSystemText(request.System)))
	}
	h.Write([]byte("|h:"))
	if b, err := json.Marshal(request.Messages[0]); err == nil {
		h.Write(b)
	}
	return "affinity:v1:" + hex.EncodeToString(h.Sum(nil))[:32], true
}

func (c *Cache) Key(request *cif.CanonicalRequest, requestedModel string) (string, bool) {
	return c.key(request, requestedModel)
}

// Lookup returns the pinned instance ID for request's conversation prefix.
func (c *Cache) Lookup(request *cif.CanonicalRequest, requestedModel string) (string, bool) {
	if !c.Enabled() {
		return "", false
	}
	k, ok := c.key(request, requestedModel)
	if !ok {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, found := c.data[k]
	if !found {
		c.misses++
		return "", false
	}
	if time.Now().After(e.expiresAt) {
		delete(c.data, k)
		c.misses++
		return "", false
	}
	c.hits++
	return e.instanceID, true
}

// Record pins instanceID for this conversation's stable head key. Called after
// a successful dispatch so subsequent turns of the same conversation reuse the
// same instance.
func (c *Cache) Record(request *cif.CanonicalRequest, requestedModel, instanceID string) {
	if !c.Enabled() || instanceID == "" {
		return
	}
	k, ok := c.key(request, requestedModel)
	if !ok {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.data[k]; !exists {
		c.order = append(c.order, k)
	}
	c.data[k] = entry{instanceID: instanceID, expiresAt: time.Now().Add(c.cfg.TTL)}
	c.evictLocked()
}

// evictLocked prunes oldest entries when over capacity. Caller holds mu.
func (c *Cache) evictLocked() {
	for len(c.data) > c.cfg.MaxEntries && len(c.order) > 0 {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.data, oldest)
	}
}
