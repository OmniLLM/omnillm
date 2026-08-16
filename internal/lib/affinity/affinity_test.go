package affinity

import (
	"testing"
	"time"

	"omnillm/internal/cif"
)

func strptr(s string) *string { return &s }

var _ = strptr

func userMsg(text string) cif.CIFMessage {
	return cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: text}}}
}

func assistantMsg(text string) cif.CIFMessage {
	return cif.CIFAssistantMessage{Role: "assistant", Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: text}}}
}

func newTestCache() *Cache {
	return NewCache(Config{Enabled: true, TTL: time.Minute, MaxEntries: 100, IncludeUserID: true})
}

// A realistic multi-turn loop: each turn appends (assistant reply, next user
// message) to the prior request. Affinity keys on the stable conversation head
// (first message), so every turn after the first hits the recorded instance.
func TestMultiTurnAffinityHit(t *testing.T) {
	c := newTestCache()
	model := "claude-opus-4"

	// Turn 1: [user]. Head exists from the start; record it.
	msgs := []cif.CIFMessage{userMsg("q1")}
	req := &cif.CanonicalRequest{Model: model, Messages: msgs}
	c.Record(req, model, "inst-A")

	// Simulate 4 more turns; each must hit.
	for turn := 2; turn <= 5; turn++ {
		msgs = append(msgs, assistantMsg("a"), userMsg("q"))
		req = &cif.CanonicalRequest{Model: model, Messages: msgs}
		got, ok := c.Lookup(req, model)
		if !ok || got != "inst-A" {
			t.Fatalf("turn %d should hit inst-A, got=%q ok=%v", turn, got, ok)
		}
		c.Record(req, model, "inst-A")
	}
}

// The precise invariant: Lookup(req) hits when req.prefix == a previously
// Recorded req.messages.
func TestPrefixEqualsPreviousFullMessages(t *testing.T) {
	c := newTestCache()
	model := "gpt-5"

	prev := &cif.CanonicalRequest{Model: model, Messages: []cif.CIFMessage{
		userMsg("a"), assistantMsg("b"),
	}}
	c.Record(prev, model, "inst-X")

	// next request's prefix (all but last) == prev.messages exactly.
	next := &cif.CanonicalRequest{Model: model, Messages: []cif.CIFMessage{
		userMsg("a"), assistantMsg("b"), userMsg("c"),
	}}
	if got, ok := c.Lookup(next, model); !ok || got != "inst-X" {
		t.Fatalf("expected hit inst-X, got=%q ok=%v", got, ok)
	}
}

func TestPromptCacheAffinityIgnoresSuffixAfterBreakpoint(t *testing.T) {
	cache := newTestCache()
	control := &cif.CIFCacheControl{Type: "ephemeral"}
	prefix := cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{
		cif.CIFTextPart{Type: "text", Text: "stable", CacheControl: control},
	}}
	first := &cif.CanonicalRequest{Model: "m", Messages: []cif.CIFMessage{prefix, userMsg("suffix-a")}}
	second := &cif.CanonicalRequest{Model: "m", Messages: []cif.CIFMessage{prefix, userMsg("suffix-b")}}
	cache.Record(first, "m", "inst-cache")
	if got, ok := cache.Lookup(second, "m"); !ok || got != "inst-cache" {
		t.Fatalf("varying suffix missed affinity: got=%q ok=%v", got, ok)
	}
}

func TestPromptCacheAffinityIgnoresSuffixWithinMarkedMessage(t *testing.T) {
	cache := newTestCache()
	control := &cif.CIFCacheControl{Type: "ephemeral"}
	request := func(suffix string) *cif.CanonicalRequest {
		return &cif.CanonicalRequest{Model: "m", Messages: []cif.CIFMessage{
			cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{
				cif.CIFTextPart{Type: "text", Text: "stable", CacheControl: control},
				cif.CIFTextPart{Type: "text", Text: suffix},
			}},
		}}
	}
	cache.Record(request("suffix-a"), "m", "inst-cache")
	if got, ok := cache.Lookup(request("suffix-b"), "m"); !ok || got != "inst-cache" {
		t.Fatalf("multipart varying suffix missed affinity: got=%q ok=%v", got, ok)
	}
}

func TestPromptCacheAffinityChangesWithPrefix(t *testing.T) {
	cache := newTestCache()
	control := &cif.CIFCacheControl{Type: "ephemeral"}
	first := &cif.CanonicalRequest{Model: "m", Messages: []cif.CIFMessage{
		cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "stable-a", CacheControl: control}}},
	}}
	second := &cif.CanonicalRequest{Model: "m", Messages: []cif.CIFMessage{
		cif.CIFUserMessage{Role: "user", Content: []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: "stable-b", CacheControl: control}}},
	}}
	cache.Record(first, "m", "inst-cache")
	if _, ok := cache.Lookup(second, "m"); ok {
		t.Fatal("changed cacheable prefix unexpectedly hit affinity")
	}
}

func TestDisabledNoOp(t *testing.T) {
	c := NewCache(Config{Enabled: false, TTL: time.Minute, MaxEntries: 10})
	req := &cif.CanonicalRequest{Model: "m", Messages: []cif.CIFMessage{userMsg("a"), assistantMsg("b")}}
	c.Record(req, "m", "inst")
	nxt := &cif.CanonicalRequest{Model: "m", Messages: []cif.CIFMessage{userMsg("a"), assistantMsg("b"), userMsg("c")}}
	if _, ok := c.Lookup(nxt, "m"); ok {
		t.Fatal("disabled cache must never hit")
	}
}

func TestTTLExpiry(t *testing.T) {
	c := NewCache(Config{Enabled: true, TTL: 10 * time.Millisecond, MaxEntries: 10})
	prev := &cif.CanonicalRequest{Model: "m", Messages: []cif.CIFMessage{userMsg("a"), assistantMsg("b")}}
	c.Record(prev, "m", "inst-T")
	next := &cif.CanonicalRequest{Model: "m", Messages: []cif.CIFMessage{userMsg("a"), assistantMsg("b"), userMsg("c")}}
	if _, ok := c.Lookup(next, "m"); !ok {
		t.Fatal("should hit before expiry")
	}
	time.Sleep(20 * time.Millisecond)
	if _, ok := c.Lookup(next, "m"); ok {
		t.Fatal("should miss after TTL expiry")
	}
}

func TestCrossModelIsolation(t *testing.T) {
	c := newTestCache()
	prev := &cif.CanonicalRequest{Model: "model-A", Messages: []cif.CIFMessage{userMsg("a"), assistantMsg("b")}}
	c.Record(prev, "model-A", "inst-A")
	// Same conversation, different model -> different key -> miss.
	next := &cif.CanonicalRequest{Model: "model-B", Messages: []cif.CIFMessage{userMsg("a"), assistantMsg("b"), userMsg("c")}}
	if _, ok := c.Lookup(next, "model-B"); ok {
		t.Fatal("different model must not share affinity")
	}
}

func TestEvictionBounded(t *testing.T) {
	c := NewCache(Config{Enabled: true, TTL: time.Hour, MaxEntries: 5})
	for i := 0; i < 50; i++ {
		req := &cif.CanonicalRequest{Model: "m", Messages: []cif.CIFMessage{userMsg(string(rune('a' + i))), assistantMsg("x")}}
		c.Record(req, "m", "inst")
	}
	_, _, size := c.Stats()
	if size > 5 {
		t.Fatalf("size %d exceeds MaxEntries 5", size)
	}
}
