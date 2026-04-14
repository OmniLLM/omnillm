// Package cif provides the Canonical Interface Format (CIF) types
// for normalizing requests and responses across different providers
package cif

import (
	"encoding/json"
	"errors"
)

// ─────────────────────────────────────────────────────────
// Canonical content part types
// ─────────────────────────────────────────────────────────

type CIFTextPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type CIFImagePart struct {
	Type      string  `json:"type"`
	MediaType string  `json:"mediaType"`
	Data      *string `json:"data,omitempty"` // base64 encoded bytes
	URL       *string `json:"url,omitempty"`  // data: URI or https URL
}

type CIFThinkingPart struct {
	Type      string  `json:"type"`
	Thinking  string  `json:"thinking"`
	Signature *string `json:"signature,omitempty"` // Antigravity thoughtSignature
}

type CIFToolCallPart struct {
	Type          string                 `json:"type"`
	ToolCallID    string                 `json:"toolCallId"`
	ToolName      string                 `json:"toolName"`
	ToolArguments map[string]interface{} `json:"toolArguments"`
}

type CIFToolResultPart struct {
	Type       string `json:"type"`
	ToolCallID string `json:"toolCallId"`
	ToolName   string `json:"toolName"` // name is needed by Antigravity functionResponse
	Content    string `json:"content"`  // serialized result text
	IsError    *bool  `json:"isError,omitempty"`
}

type CIFContentPart interface {
	GetType() string
}

func (p CIFTextPart) GetType() string       { return "text" }
func (p CIFImagePart) GetType() string      { return "image" }
func (p CIFThinkingPart) GetType() string   { return "thinking" }
func (p CIFToolCallPart) GetType() string   { return "tool_call" }
func (p CIFToolResultPart) GetType() string { return "tool_result" }

// ─────────────────────────────────────────────────────────
// Canonical message types
// ─────────────────────────────────────────────────────────

type CIFSystemMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"` // always plain text; multiple system blocks are joined
}

type CIFUserMessage struct {
	Role    string           `json:"role"`
	Content []CIFContentPart `json:"content"` // may include text, image, tool_result parts
}

type CIFAssistantMessage struct {
	Role    string           `json:"role"`
	Content []CIFContentPart `json:"content"` // may include text, thinking, tool_call parts
}

type CIFMessage interface {
	GetRole() string
}

func (m CIFSystemMessage) GetRole() string    { return "system" }
func (m CIFUserMessage) GetRole() string      { return "user" }
func (m CIFAssistantMessage) GetRole() string { return "assistant" }

// ─────────────────────────────────────────────────────────
// Canonical tool definition
// ─────────────────────────────────────────────────────────

type CIFTool struct {
	Name             string                 `json:"name"`
	Description      *string                `json:"description,omitempty"`
	ParametersSchema map[string]interface{} `json:"parametersSchema"` // JSON Schema object
}

type CIFToolChoice interface{}

// Can be "none", "auto", "required", or {"type": "function", "functionName": "..."}

// ─────────────────────────────────────────────────────────
// Canonical request
// ─────────────────────────────────────────────────────────

type CanonicalRequest struct {
	// Model name exactly as the caller specified it, before any provider remapping
	Model string `json:"model"`

	// System prompt, if any. Placed before messages.
	SystemPrompt *string `json:"systemPrompt,omitempty"`

	// Ordered conversation history
	Messages []CIFMessage `json:"messages"`

	// Tools the model may call
	Tools      []CIFTool     `json:"tools,omitempty"`
	ToolChoice CIFToolChoice `json:"toolChoice,omitempty"`

	// Sampling parameters — all optional
	Temperature *float64 `json:"temperature,omitempty"`
	TopP        *float64 `json:"topP,omitempty"`
	MaxTokens   *int     `json:"maxTokens,omitempty"`
	Stop        []string `json:"stop,omitempty"` // always array; adapters normalise string→[string]
	Stream      bool     `json:"stream"`         // required, default false

	// Pass-through metadata (forwarded to provider if understood)
	UserID *string `json:"userId,omitempty"`

	// Incoming request headers for trace logging (not serialized to JSON)
	IncomingHeaders map[string]string `json:"-"`

	// Extended / provider-specific hints (type-safe escape hatch)
	Extensions *Extensions `json:"extensions,omitempty"`
}

type Extensions struct {
	// Anthropic thinking budget
	ThinkingBudgetTokens *int `json:"thinkingBudgetTokens,omitempty"`
	// Qwen needs dummy tool injection — flag set by Alibaba adapter
	RequiresDummyToolInjection *bool `json:"requiresDummyToolInjection,omitempty"`
	// Force providers that support multiple upstream APIs to stay on chat completions.
	ForceChatCompletions *bool `json:"forceChatCompletions,omitempty"`
	// Disable provider-level auth retries to keep one upstream attempt per user request.
	DisableAuthRetry *bool `json:"disableAuthRetry,omitempty"`
	// Disable route-level streaming fallback to a second non-streaming request.
	DisableStreamingFallback *bool `json:"disableStreamingFallback,omitempty"`
}

// ─────────────────────────────────────────────────────────
// Canonical response
// ─────────────────────────────────────────────────────────

type CIFStopReason string

const (
	StopReasonEndTurn       CIFStopReason = "end_turn"       // natural stop
	StopReasonMaxTokens     CIFStopReason = "max_tokens"     // hit token limit
	StopReasonToolUse       CIFStopReason = "tool_use"       // model wants to call a tool
	StopReasonStopSequence  CIFStopReason = "stop_sequence"  // matched a stop string
	StopReasonContentFilter CIFStopReason = "content_filter" // safety filter
	StopReasonError         CIFStopReason = "error"
)

type CIFUsage struct {
	InputTokens           int  `json:"inputTokens"`
	OutputTokens          int  `json:"outputTokens"`
	CacheReadInputTokens  *int `json:"cacheReadInputTokens,omitempty"`
	CacheWriteInputTokens *int `json:"cacheWriteInputTokens,omitempty"`
}

type CanonicalResponse struct {
	// Provider-assigned response ID
	ID string `json:"id"`
	// The model name reported by the provider (may differ from request model)
	Model string `json:"model"`
	// All generated content, in order
	Content      []CIFContentPart `json:"content"` // text, thinking, tool_call parts
	StopReason   CIFStopReason    `json:"stopReason"`
	StopSequence *string          `json:"stopSequence,omitempty"`
	Usage        *CIFUsage        `json:"usage,omitempty"`
}

// ─────────────────────────────────────────────────────────
// Streaming canonical events
// ─────────────────────────────────────────────────────────

type CIFStreamStart struct {
	Type  string `json:"type"`
	ID    string `json:"id"`
	Model string `json:"model"`
}

type CIFContentDelta struct {
	Type         string         `json:"type"`
	Index        int            `json:"index"`
	ContentBlock CIFContentPart `json:"contentBlock,omitempty"` // For new blocks: includes full part shape with empty/partial content
	Delta        DeltaContent   `json:"delta"`                  // Delta for text or tool argument accumulation
}

type DeltaContent interface {
	GetDeltaType() string
}

type TextDelta struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type ThinkingDelta struct {
	Type     string `json:"type"`
	Thinking string `json:"thinking"`
}

type ToolArgumentsDelta struct {
	Type        string `json:"type"`
	PartialJSON string `json:"partialJson"`
}

func (d TextDelta) GetDeltaType() string          { return "text_delta" }
func (d ThinkingDelta) GetDeltaType() string      { return "thinking_delta" }
func (d ToolArgumentsDelta) GetDeltaType() string { return "tool_arguments_delta" }

type CIFContentBlockStop struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
}

type CIFStreamEnd struct {
	Type         string        `json:"type"`
	StopReason   CIFStopReason `json:"stopReason"`
	StopSequence *string       `json:"stopSequence,omitempty"`
	Usage        *CIFUsage     `json:"usage,omitempty"`
}

type CIFStreamError struct {
	Type  string    `json:"type"`
	Error ErrorInfo `json:"error"`
}

type ErrorInfo struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type CIFStreamEvent interface {
	GetEventType() string
}

func (e CIFStreamStart) GetEventType() string      { return "stream_start" }
func (e CIFContentDelta) GetEventType() string     { return "content_delta" }
func (e CIFContentBlockStop) GetEventType() string { return "content_block_stop" }
func (e CIFStreamEnd) GetEventType() string        { return "stream_end" }
func (e CIFStreamError) GetEventType() string      { return "stream_error" }

// ─────────────────────────────────────────────────────────
// Helper types for JSON marshaling
// ─────────────────────────────────────────────────────────

// ContentPartJSON is used for JSON marshaling/unmarshaling
type ContentPartJSON struct {
	Type          string                 `json:"type"`
	Text          string                 `json:"text,omitempty"`
	MediaType     string                 `json:"mediaType,omitempty"`
	Data          *string                `json:"data,omitempty"`
	URL           *string                `json:"url,omitempty"`
	Thinking      string                 `json:"thinking,omitempty"`
	Signature     *string                `json:"signature,omitempty"`
	ToolCallID    string                 `json:"toolCallId,omitempty"`
	ToolName      string                 `json:"toolName,omitempty"`
	ToolArguments map[string]interface{} `json:"toolArguments,omitempty"`
	Content       string                 `json:"content,omitempty"`
	IsError       *bool                  `json:"isError,omitempty"`
}

// Custom JSON marshaling for content parts
func MarshalCIFContentPart(p CIFContentPart) ([]byte, error) {
	switch part := p.(type) {
	case CIFTextPart:
		return json.Marshal(ContentPartJSON{
			Type: "text",
			Text: part.Text,
		})
	case CIFImagePart:
		return json.Marshal(ContentPartJSON{
			Type:      "image",
			MediaType: part.MediaType,
			Data:      part.Data,
			URL:       part.URL,
		})
	case CIFThinkingPart:
		return json.Marshal(ContentPartJSON{
			Type:      "thinking",
			Thinking:  part.Thinking,
			Signature: part.Signature,
		})
	case CIFToolCallPart:
		return json.Marshal(ContentPartJSON{
			Type:          "tool_call",
			ToolCallID:    part.ToolCallID,
			ToolName:      part.ToolName,
			ToolArguments: part.ToolArguments,
		})
	case CIFToolResultPart:
		return json.Marshal(ContentPartJSON{
			Type:       "tool_result",
			ToolCallID: part.ToolCallID,
			ToolName:   part.ToolName,
			Content:    part.Content,
			IsError:    part.IsError,
		})
	default:
		return nil, errors.New("unknown content part type")
	}
}
