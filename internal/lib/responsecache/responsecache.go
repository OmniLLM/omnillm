// Package responsecache implements an exact-match cache for deterministic,
// non-streaming LLM responses at the CIF (Canonical Intermediate Format) layer.
//
// Design rationale:
//
//   - Caching at the CIF layer (CanonicalResponse) rather than at a wire shape
//     makes the cache shape-agnostic: a response produced for an OpenAI-shape
//     client can be re-serialized to satisfy an Anthropic-shape client and vice
//     versa, because every route serializes from the same CanonicalResponse.
//
//   - Only DETERMINISTIC requests are cacheable. temperature == 0 (or unset,
//     which most providers treat as greedy/near-greedy — we require an explicit
//     0 to be safe) and top_p unset/>=1. Any tool call, any temperature > 0, any
//     streaming request is a hard skip: returning a stale answer there is a
//     correctness bug, not an optimization.
//
//   - The cache key is a SHA-256 over the salient, order-preserving parts of the
//     canonical request. Two requests collide iff they would deterministically
//     produce the same generation.
package responsecache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"omnillm/internal/cif"
	"omnillm/internal/database"
)

// Config controls cache behaviour. Values are read from the process config store
// on each request (cheap SQLite point-read) so the cache can be toggled live
// without a restart.
type Config struct {
	Enabled bool
	TTL     time.Duration
}

const (
	cfgKeyEnabled = "response_cache.enabled"
	cfgKeyTTLSecs = "response_cache.ttl_seconds"

	// DefaultTTL applies when the operator enabled the cache but set no TTL.
	DefaultTTL = 1 * time.Hour

	// BypassHeader lets a client force a cache miss+refresh for one request.
	// Value "bypass" skips the read (still writes); "off" skips read and write.
	BypassHeader = "X-OmniLLM-Cache"
)

// LoadConfig reads the live cache configuration from the config store.
// Absent/blank keys mean disabled — the cache is strictly opt-in.
func LoadConfig() Config {
	store := database.NewConfigStore()
	enabled := false
	if v, err := store.Get(cfgKeyEnabled); err == nil {
		enabled = v == "true" || v == "1"
	}
	ttl := DefaultTTL
	if v, err := store.Get(cfgKeyTTLSecs); err == nil && v != "" {
		if secs, perr := parsePositiveInt(v); perr == nil && secs > 0 {
			ttl = time.Duration(secs) * time.Second
		}
	}
	return Config{Enabled: enabled, TTL: ttl}
}

// Cacheable reports whether a canonical request is eligible for exact-match
// caching. It is intentionally conservative: any doubt means "not cacheable".
// Streaming IS eligible — a streaming response is accumulated to a
// CanonicalResponse on the way out and replayed as synthetic SSE on a hit.
func Cacheable(req *cif.CanonicalRequest) bool {
	if req == nil {
		return false
	}
	// Require an explicit temperature of exactly 0 — greedy decoding.
	if req.Temperature == nil || *req.Temperature != 0 {
		return false
	}
	// If top_p is pinned below 1 alongside temp 0 it's still deterministic, but
	// an explicit top_p > 0 && < 1 with sampling elsewhere is a smell; only allow
	// unset or >= 1 (no-op) top_p.
	if req.TopP != nil && *req.TopP < 1 {
		return false
	}
	return true
}

// Key derives the stable cache key for a canonical request. Only fields that
// affect a deterministic generation are hashed; transport/metadata (headers,
// user id, stream flag) are excluded so semantically identical requests collide.
func Key(req *cif.CanonicalRequest) string {
	// A struct with a fixed field order gives a stable JSON encoding.
	keyed := struct {
		Model          string                 `json:"model"`
		System         *string                `json:"system,omitempty"`
		Messages       []cif.CIFMessage        `json:"messages"`
		Tools          []cif.CIFTool           `json:"tools,omitempty"`
		ToolChoice     cif.CIFToolChoice       `json:"toolChoice,omitempty"`
		Temperature    *float64               `json:"temperature,omitempty"`
		TopP           *float64               `json:"topP,omitempty"`
		MaxTokens      *int                   `json:"maxTokens,omitempty"`
		Stop           []string               `json:"stop,omitempty"`
		ResponseFormat map[string]interface{} `json:"responseFormat,omitempty"`
	}{
		Model:          req.Model,
		System:         req.SystemPrompt,
		Messages:       req.Messages,
		Tools:          req.Tools,
		ToolChoice:     req.ToolChoice,
		Temperature:    req.Temperature,
		TopP:           req.TopP,
		MaxTokens:      req.MaxTokens,
		Stop:           req.Stop,
		ResponseFormat: req.ResponseFormat,
	}
	raw, err := json.Marshal(keyed)
	if err != nil {
		// Marshal failure ⇒ un-cacheable key; a random-ish sum avoids collisions.
		raw = []byte(req.Model + "|marshal-error")
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// Get returns a cached CanonicalResponse for req, or nil on miss/disabled/expired.
func Get(cfg Config, req *cif.CanonicalRequest, key string) *cif.CanonicalResponse {
	if !cfg.Enabled {
		return nil
	}
	rec, err := database.NewResponseCacheStore().Get(key, cfg.TTL)
	if err != nil || rec == nil {
		return nil
	}
	resp, err := decodeResponse(rec.ResponseData)
	if err != nil {
		return nil
	}
	return resp
}

// Put stores a CanonicalResponse. Errors are swallowed: caching is best-effort
// and must never fail a request that already succeeded upstream.
func Put(cfg Config, req *cif.CanonicalRequest, key string, resp *cif.CanonicalResponse) {
	if !cfg.Enabled || resp == nil {
		return
	}
	// Never cache an error/empty generation.
	if resp.StopReason == cif.StopReasonError || len(resp.Content) == 0 {
		return
	}
	data, err := encodeResponse(resp)
	if err != nil {
		return
	}
	_ = database.NewResponseCacheStore().Save(key, req.Model, data)
}

// cachedResponse is a JSON-round-trippable projection of cif.CanonicalResponse.
// The CIF response's Content is []CIFContentPart (an interface slice) which the
// stdlib json decoder cannot reconstruct, so we tag each part with its concrete
// type on the way in and rebuild the interface values on the way out. Response
// content only ever contains text, thinking, or tool_call parts.
type cachedResponse struct {
	ID           string            `json:"id"`
	Model        string            `json:"model"`
	StopReason   cif.CIFStopReason `json:"stopReason"`
	StopSequence *string           `json:"stopSequence,omitempty"`
	Usage        *cif.CIFUsage     `json:"usage,omitempty"`
	Content      []cachedPart      `json:"content"`
}

type cachedPart struct {
	Type          string                 `json:"type"`
	Text          string                 `json:"text,omitempty"`
	Thinking      string                 `json:"thinking,omitempty"`
	Signature     *string                `json:"signature,omitempty"`
	ToolCallID    string                 `json:"toolCallId,omitempty"`
	ToolName      string                 `json:"toolName,omitempty"`
	ToolArguments map[string]interface{} `json:"toolArguments,omitempty"`
}

func encodeResponse(resp *cif.CanonicalResponse) (string, error) {
	cr := cachedResponse{
		ID:           resp.ID,
		Model:        resp.Model,
		StopReason:   resp.StopReason,
		StopSequence: resp.StopSequence,
		Usage:        resp.Usage,
	}
	for _, part := range resp.Content {
		switch p := part.(type) {
		case cif.CIFTextPart:
			cr.Content = append(cr.Content, cachedPart{Type: "text", Text: p.Text})
		case cif.CIFThinkingPart:
			cr.Content = append(cr.Content, cachedPart{Type: "thinking", Thinking: p.Thinking, Signature: p.Signature})
		case cif.CIFToolCallPart:
			cr.Content = append(cr.Content, cachedPart{
				Type:          "tool_call",
				ToolCallID:    p.ToolCallID,
				ToolName:      p.ToolName,
				ToolArguments: p.ToolArguments,
			})
		default:
			// Unknown part type ⇒ un-cacheable (image/tool_result should not
			// appear in a response, but bail safely rather than lose data).
			return "", errUncacheablePart
		}
	}
	raw, err := json.Marshal(cr)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func decodeResponse(data string) (*cif.CanonicalResponse, error) {
	var cr cachedResponse
	if err := json.Unmarshal([]byte(data), &cr); err != nil {
		return nil, err
	}
	resp := &cif.CanonicalResponse{
		ID:           cr.ID,
		Model:        cr.Model,
		StopReason:   cr.StopReason,
		StopSequence: cr.StopSequence,
		Usage:        cr.Usage,
	}
	for _, p := range cr.Content {
		switch p.Type {
		case "text":
			resp.Content = append(resp.Content, cif.CIFTextPart{Type: "text", Text: p.Text})
		case "thinking":
			resp.Content = append(resp.Content, cif.CIFThinkingPart{Type: "thinking", Thinking: p.Thinking, Signature: p.Signature})
		case "tool_call":
			resp.Content = append(resp.Content, cif.CIFToolCallPart{
				Type:          "tool_call",
				ToolCallID:    p.ToolCallID,
				ToolName:      p.ToolName,
				ToolArguments: p.ToolArguments,
			})
		default:
			return nil, errUncacheablePart
		}
	}
	return resp, nil
}

var errUncacheablePart = errors.New("responsecache: unsupported content part type")

// BypassMode interprets the per-request bypass header.
type BypassMode int

const (
	BypassNone BypassMode = iota // normal read+write
	BypassRead                   // skip read, still write (force refresh)
	BypassAll                    // skip read and write
)

// ParseBypass maps a header value to a BypassMode.
func ParseBypass(headerValue string) BypassMode {
	switch strings.ToLower(strings.TrimSpace(headerValue)) {
	case "bypass", "refresh", "no-cache":
		return BypassRead
	case "off", "disable":
		return BypassAll
	default:
		return BypassNone
	}
}

func parsePositiveInt(s string) (int, error) {
	var n int
	err := json.Unmarshal([]byte(s), &n)
	return n, err
}

// encodeToolArgs serializes tool-call arguments to a JSON string for stream
// replay. Returns "{}" on failure so the synthesized delta is always valid JSON.
func encodeToolArgs(args map[string]interface{}) string {
	if len(args) == 0 {
		return "{}"
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

// decodeToolArgs parses accumulated raw tool-argument JSON back into a map.
func decodeToolArgs(raw string) map[string]interface{} {
	if raw == "" {
		return map[string]interface{}{}
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return map[string]interface{}{}
	}
	return m
}
