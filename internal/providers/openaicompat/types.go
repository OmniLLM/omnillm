// OpenAI-compatible wire types.
// Kept in this package so every provider that speaks the OpenAI chat
// completions protocol can share the same serialization layer without
// importing each other.
package openaicompat

// ─── Outbound request ─────────────────────────────────────────────────────────

// ChatRequest is the JSON body sent to any OpenAI-compatible /chat/completions
// endpoint.
type ChatRequest struct {
	Model         string         `json:"model"`
	Messages      []Message      `json:"messages"`
	Stream        bool           `json:"stream"`
	Temperature   *float64       `json:"temperature,omitempty"`
	TopP          *float64       `json:"top_p,omitempty"`
	MaxTokens     *int           `json:"max_tokens,omitempty"`
	Stop          []string       `json:"stop,omitempty"`
	User          *string        `json:"user,omitempty"`
	Tools         []Tool         `json:"tools,omitempty"`
	ToolChoice    interface{}    `json:"tool_choice,omitempty"`
	StreamOptions *StreamOptions `json:"stream_options,omitempty"`

	// Provider-specific extension fields — set by each provider's
	// BuildExtra hook, marshalled into the final JSON via Extras map.
	// Use Extras instead of embedding unknown fields directly.
	Extras map[string]interface{} `json:"-"`
}

// StreamOptions requests per-chunk usage stats in SSE responses.
type StreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// Message is one conversation turn (system / user / assistant / tool).
type Message struct {
	Role             string      `json:"role"`
	Content          interface{} `json:"content"` // string or []ContentPart
	ToolCalls        []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID       string      `json:"tool_call_id,omitempty"`
	ReasoningContent string      `json:"reasoning_content,omitempty"` // Qwen3 / o1-style
}

// ContentPart is one element of a multipart content array.
type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL wraps a base64 data-URI or HTTPS image URL.
type ImageURL struct {
	URL string `json:"url"`
}

// Tool is an OpenAI-compatible function tool definition.
type Tool struct {
	Type     string       `json:"type"`
	Function FunctionSpec `json:"function"`
}

// FunctionSpec describes a callable function.
type FunctionSpec struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ToolCall is a model-requested function invocation in an assistant message.
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function FunctionCallSpec `json:"function"`
}

// FunctionCallSpec carries the name and JSON-encoded arguments of a tool call.
type FunctionCallSpec struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON-encoded string
}

// ─── Inbound response ─────────────────────────────────────────────────────────

// ChatResponse is the JSON body returned by a non-streaming request.
type ChatResponse struct {
	ID      string   `json:"id"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   *Usage   `json:"usage,omitempty"`
}

// Choice is one candidate completion in the response.
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// Usage contains token-count statistics.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ─── Streaming chunks ─────────────────────────────────────────────────────────

// StreamChunk is one SSE data payload in a streaming response.
type StreamChunk struct {
	ID      string         `json:"id"`
	Model   string         `json:"model"`
	Choices []StreamChoice `json:"choices"`
	Usage   *Usage         `json:"usage,omitempty"`
}

// StreamChoice is one streaming candidate within a chunk.
type StreamChoice struct {
	Index        int          `json:"index"`
	Delta        MessageDelta `json:"delta"`
	FinishReason string       `json:"finish_reason"`
}

// MessageDelta carries the incremental content within a streaming chunk.
type MessageDelta struct {
	Role             string          `json:"role,omitempty"`
	Content          string          `json:"content,omitempty"`
	ReasoningContent string          `json:"reasoning_content,omitempty"` // Qwen3 thinking
	ToolCalls        []ToolCallDelta `json:"tool_calls,omitempty"`
}

// ToolCallDelta is an incremental tool-call update in a streaming chunk.
type ToolCallDelta struct {
	Index    int                   `json:"index"`
	ID       string                `json:"id,omitempty"`
	Type     string                `json:"type,omitempty"`
	Function FunctionCallDeltaSpec `json:"function"`
}

// FunctionCallDeltaSpec carries partial function-call data in a streaming chunk.
type FunctionCallDeltaSpec struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"` // partial JSON
}
