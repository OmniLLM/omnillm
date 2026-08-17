// Package responsecache implements an exact-match cache for LLM responses at
// the CIF (Canonical Intermediate Format) layer.
//
// Responses are stored canonically so cache entries can be re-serialized into
// the current caller's dialect and streaming mode. Cache identity includes
// generation semantics while excluding transport, routing, dialect, and
// provider prompt-cache controls.
package responsecache

import (
	"bytes"
	"context"
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
	DefaultTTL = 60 * time.Second

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

// Key derives the stable semantic cache key for a canonical request. It returns
// an error and no key when semantic material cannot be serialized safely.
func Key(req *cif.CanonicalRequest) (string, error) {
	if req == nil {
		return "", errors.New("responsecache: nil request")
	}

	keyed := struct {
		Model              string                 `json:"model"`
		System             []cif.CIFSystemBlock   `json:"system,omitempty"`
		Messages           []cif.CIFMessage       `json:"messages"`
		Tools              []cif.CIFTool          `json:"tools,omitempty"`
		ToolChoice         cif.CIFToolChoice      `json:"toolChoice,omitempty"`
		Temperature        *float64               `json:"temperature,omitempty"`
		TopP               *float64               `json:"topP,omitempty"`
		MaxTokens          *int                   `json:"maxTokens,omitempty"`
		Stop               []string               `json:"stop,omitempty"`
		ResponseFormat     map[string]interface{} `json:"responseFormat,omitempty"`
		PreviousResponseID *string                `json:"previousResponseId,omitempty"`
		UserID             *string                `json:"userId,omitempty"`
		ThinkingBudget     *int                   `json:"thinkingBudgetTokens,omitempty"`
	}{
		Model:              req.Model,
		System:             semanticSystem(req.System),
		Messages:           semanticMessages(req.Messages),
		Tools:              semanticTools(req.Tools),
		ToolChoice:         req.ToolChoice,
		Temperature:        req.Temperature,
		TopP:               req.TopP,
		MaxTokens:          req.MaxTokens,
		Stop:               req.Stop,
		ResponseFormat:     req.ResponseFormat,
		PreviousResponseID: req.PreviousResponseID,
		UserID:             req.UserID,
	}
	if req.Extensions != nil {
		keyed.ThinkingBudget = req.Extensions.ThinkingBudgetTokens
	}

	raw, err := json.Marshal(keyed)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func semanticSystem(system []cif.CIFSystemBlock) []cif.CIFSystemBlock {
	if system == nil {
		return nil
	}
	result := make([]cif.CIFSystemBlock, len(system))
	for i, block := range system {
		result[i] = cif.CIFSystemBlock{Type: block.Type, Text: block.Text}
	}
	return result
}

func semanticTools(tools []cif.CIFTool) []cif.CIFTool {
	if tools == nil {
		return nil
	}
	result := make([]cif.CIFTool, len(tools))
	for i, tool := range tools {
		result[i] = tool
		result[i].CacheControl = nil
	}
	return result
}

func semanticMessages(messages []cif.CIFMessage) []cif.CIFMessage {
	if messages == nil {
		return nil
	}
	result := make([]cif.CIFMessage, 0, len(messages))
	for _, message := range messages {
		switch typed := message.(type) {
		case cif.CIFSystemMessage:
			result = append(result, cif.CIFSystemMessage{Role: typed.Role, Content: semanticSystem(typed.Content)})
		case cif.CIFUserMessage:
			result = append(result, cif.CIFUserMessage{Role: typed.Role, Content: semanticContent(typed.Content)})
		case cif.CIFAssistantMessage:
			result = append(result, cif.CIFAssistantMessage{Role: typed.Role, Content: semanticContent(typed.Content)})
		default:
			// Keep unknown implementations in the projection so unsupported values
			// fail closed during marshaling rather than silently losing semantics.
			result = append(result, message)
		}
	}
	return result
}

func semanticContent(parts []cif.CIFContentPart) []cif.CIFContentPart {
	if parts == nil {
		return nil
	}
	result := make([]cif.CIFContentPart, 0, len(parts))
	for _, part := range parts {
		switch typed := part.(type) {
		case cif.CIFTextPart:
			typed.CacheControl = nil
			result = append(result, typed)
		case cif.CIFImagePart:
			typed.CacheControl = nil
			result = append(result, typed)
		case cif.CIFThinkingPart:
			typed.CacheControl = nil
			result = append(result, typed)
		case cif.CIFToolCallPart:
			typed.CacheControl = nil
			result = append(result, typed)
		case cif.CIFToolResultPart:
			typed.CacheControl = nil
			result = append(result, typed)
		default:
			result = append(result, part)
		}
	}
	return result
}

// GetContext returns a cached CanonicalResponse and propagates ctx to storage.
// Backend failures and malformed entries fail open as cache misses.
func GetContext(ctx context.Context, cfg Config, req *cif.CanonicalRequest, key string) *cif.CanonicalResponse {
	if !cfg.Enabled {
		return nil
	}
	rec, err := CurrentStore().Get(ctx, key)
	if err != nil || rec == nil {
		return nil
	}
	resp, err := decodeResponse(rec.ResponseData)
	if err != nil {
		return nil
	}
	return resp
}

// PutContext stores a CanonicalResponse and propagates ctx to storage. Backend
// failures remain best-effort and never affect the successful upstream response.
func PutContext(ctx context.Context, cfg Config, req *cif.CanonicalRequest, key string, resp *cif.CanonicalResponse) {
	if !cfg.Enabled || req == nil || resp == nil {
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
	_ = CurrentStore().Save(ctx, key, req.Model, data, effectiveTTL(cfg.TTL))
}

func effectiveTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return DefaultTTL
	}
	return ttl
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
	ToolKind      cif.CIFToolKind        `json:"toolKind,omitempty"`
	RawInput      *string                `json:"rawInput,omitempty"`
	Namespace     string                 `json:"namespace,omitempty"`
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
				ToolKind:      p.ToolKind,
				RawInput:      p.RawInput,
				Namespace:     p.Namespace,
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
				ToolKind:      p.ToolKind,
				RawInput:      p.RawInput,
				Namespace:     p.Namespace,
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
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&m); err != nil {
		return map[string]interface{}{}
	}
	return m
}
