package shared

import (
	"io"
	"strings"
	"testing"

	"omnillm/internal/cif"
)

// sseBody wraps a string as an io.ReadCloser for use in ParseOpenAISSE tests.
func sseBody(s string) io.ReadCloser {
	return io.NopCloser(strings.NewReader(s))
}

// collectSSE drains the event channel returned by ParseOpenAISSE and returns
// all events as a slice.
func collectSSE(body io.ReadCloser) []cif.CIFStreamEvent {
	ch := make(chan cif.CIFStreamEvent, 64)
	go ParseOpenAISSE(body, ch)
	var events []cif.CIFStreamEvent
	for evt := range ch {
		events = append(events, evt)
	}
	return events
}

// ─── stop reason override ────────────────────────────────────────────────────

// TestParseOpenAISSE_ToolCallsFinishReasonStop verifies that when a provider
// (e.g. Qwen3) streams tool_calls deltas but reports finish_reason "stop"
// instead of "tool_calls", the emitted CIFStreamEnd carries StopReasonToolUse.
func TestParseOpenAISSE_ToolCallsFinishReasonStop(t *testing.T) {
	stream := `data: {"id":"r1","model":"qwen3","choices":[{"index":0,"delta":{"role":"assistant","content":null},"finish_reason":null}]}

data: {"id":"r1","model":"qwen3","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"read_file","arguments":""}}]},"finish_reason":null}]}

data: {"id":"r1","model":"qwen3","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":\"/tmp/test\"}"}}]},"finish_reason":null}]}

data: {"id":"r1","model":"qwen3","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]
`
	events := collectSSE(sseBody(stream))

	var streamEnd *cif.CIFStreamEnd
	for _, e := range events {
		if end, ok := e.(cif.CIFStreamEnd); ok {
			streamEnd = &end
		}
	}
	if streamEnd == nil {
		t.Fatal("expected CIFStreamEnd event, got none")
	}
	if streamEnd.StopReason != cif.StopReasonToolUse {
		t.Errorf("expected StopReasonToolUse, got %q", streamEnd.StopReason)
	}
}

// TestParseOpenAISSE_ToolCallsFinishReasonToolCalls verifies the standard case
// where finish_reason is "tool_calls" (non-Qwen3 providers).
func TestParseOpenAISSE_ToolCallsFinishReasonToolCalls(t *testing.T) {
	stream := `data: {"id":"r2","model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}

data: {"id":"r2","model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_xyz","type":"function","function":{"name":"search","arguments":""}}]},"finish_reason":null}]}

data: {"id":"r2","model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"q\":\"hello\"}"}}]},"finish_reason":null}]}

data: {"id":"r2","model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

data: [DONE]
`
	events := collectSSE(sseBody(stream))

	var streamEnd *cif.CIFStreamEnd
	for _, e := range events {
		if end, ok := e.(cif.CIFStreamEnd); ok {
			streamEnd = &end
		}
	}
	if streamEnd == nil {
		t.Fatal("expected CIFStreamEnd event, got none")
	}
	if streamEnd.StopReason != cif.StopReasonToolUse {
		t.Errorf("expected StopReasonToolUse, got %q", streamEnd.StopReason)
	}
}

// TestParseOpenAISSE_NoToolCallsFinishReasonStop verifies that when no tool
// calls are present and finish_reason is "stop", StopReasonEndTurn is used.
func TestParseOpenAISSE_NoToolCallsFinishReasonStop(t *testing.T) {
	stream := `data: {"id":"r3","model":"qwen3","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello!"},"finish_reason":null}]}

data: {"id":"r3","model":"qwen3","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]
`
	events := collectSSE(sseBody(stream))

	var streamEnd *cif.CIFStreamEnd
	for _, e := range events {
		if end, ok := e.(cif.CIFStreamEnd); ok {
			streamEnd = &end
		}
	}
	if streamEnd == nil {
		t.Fatal("expected CIFStreamEnd event, got none")
	}
	if streamEnd.StopReason != cif.StopReasonEndTurn {
		t.Errorf("expected StopReasonEndTurn, got %q", streamEnd.StopReason)
	}
}

// TestParseOpenAISSE_DoneWithoutFinishReason verifies that when [DONE] is
// reached without a prior finish_reason event, the stop reason is inferred
// from whether tool calls were observed.
func TestParseOpenAISSE_DoneWithoutFinishReason_NoToolCalls(t *testing.T) {
	stream := `data: {"id":"r4","model":"qwen3","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":null}]}

data: [DONE]
`
	events := collectSSE(sseBody(stream))

	var streamEnd *cif.CIFStreamEnd
	for _, e := range events {
		if end, ok := e.(cif.CIFStreamEnd); ok {
			streamEnd = &end
		}
	}
	if streamEnd == nil {
		t.Fatal("expected CIFStreamEnd event, got none")
	}
	if streamEnd.StopReason != cif.StopReasonEndTurn {
		t.Errorf("expected StopReasonEndTurn when no tools, got %q", streamEnd.StopReason)
	}
}

func TestParseOpenAISSE_DoneWithoutFinishReason_WithToolCalls(t *testing.T) {
	stream := `data: {"id":"r5","model":"qwen3","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"fn","arguments":"{}"}}]},"finish_reason":null}]}

data: [DONE]
`
	events := collectSSE(sseBody(stream))

	var streamEnd *cif.CIFStreamEnd
	for _, e := range events {
		if end, ok := e.(cif.CIFStreamEnd); ok {
			streamEnd = &end
		}
	}
	if streamEnd == nil {
		t.Fatal("expected CIFStreamEnd event, got none")
	}
	if streamEnd.StopReason != cif.StopReasonToolUse {
		t.Errorf("expected StopReasonToolUse from [DONE] with tool calls, got %q", streamEnd.StopReason)
	}
}

// ─── reasoning_content (Qwen3 thinking) ─────────────────────────────────────

// TestParseOpenAISSE_ReasoningContent verifies that delta chunks containing
// reasoning_content (Qwen3 thinking) are forwarded as ThinkingDelta events.
func TestParseOpenAISSE_ReasoningContent(t *testing.T) {
	stream := `data: {"id":"r6","model":"qwen3","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"Let me think..."},"finish_reason":null}]}

data: {"id":"r6","model":"qwen3","choices":[{"index":0,"delta":{"reasoning_content":" about this."},"finish_reason":null}]}

data: {"id":"r6","model":"qwen3","choices":[{"index":0,"delta":{"content":"The answer is 42."},"finish_reason":null}]}

data: {"id":"r6","model":"qwen3","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]
`
	events := collectSSE(sseBody(stream))

	var thinkingDeltas []string
	var textDeltas []string
	for _, e := range events {
		delta, ok := e.(cif.CIFContentDelta)
		if !ok {
			continue
		}
		switch d := delta.Delta.(type) {
		case cif.ThinkingDelta:
			thinkingDeltas = append(thinkingDeltas, d.Thinking)
		case cif.TextDelta:
			textDeltas = append(textDeltas, d.Text)
		}
	}

	if len(thinkingDeltas) != 2 {
		t.Errorf("expected 2 thinking deltas, got %d: %v", len(thinkingDeltas), thinkingDeltas)
	}
	if len(textDeltas) != 1 || textDeltas[0] != "The answer is 42." {
		t.Errorf("unexpected text deltas: %v", textDeltas)
	}

	// The first thinking delta should carry a ContentBlock (block start).
	for _, e := range events {
		delta, ok := e.(cif.CIFContentDelta)
		if !ok {
			continue
		}
		if _, isThinking := delta.Delta.(cif.ThinkingDelta); isThinking {
			if delta.ContentBlock != nil {
				// First thinking delta has ContentBlock set — correct.
				return
			}
		}
	}
	t.Error("expected first thinking delta to carry a ContentBlock for block-start signalling")
}

func TestParseOpenAISSE_ReasoningContentWithSignature(t *testing.T) {
	stream := `data: {"id":"r6sig","model":"qwen3","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"Let me think...","reasoning_signature":"sig-123"},"finish_reason":null}]}

data: {"id":"r6sig","model":"qwen3","choices":[{"index":0,"delta":{"reasoning_content":" about this."},"finish_reason":null}]}

data: {"id":"r6sig","model":"qwen3","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]
`
	events := collectSSE(sseBody(stream))

	var firstThinking *cif.CIFContentDelta
	for _, e := range events {
		delta, ok := e.(cif.CIFContentDelta)
		if !ok {
			continue
		}
		if _, isThinking := delta.Delta.(cif.ThinkingDelta); isThinking {
			if delta.ContentBlock != nil {
				firstThinking = &delta
				break
			}
		}
	}
	if firstThinking == nil {
		t.Fatal("expected first thinking delta with ContentBlock")
	}
	thinking, ok := firstThinking.ContentBlock.(cif.CIFThinkingPart)
	if !ok {
		t.Fatalf("expected CIFThinkingPart, got %T", firstThinking.ContentBlock)
	}
	if thinking.Signature == nil || *thinking.Signature != "sig-123" {
		t.Fatalf("expected signature sig-123, got %#v", thinking.Signature)
	}
}


// TestParseOpenAISSE_ToolCallDeltaEvents verifies that tool call chunks are
// correctly emitted as CIFContentDelta events with CIFToolCallPart blocks.
func TestParseOpenAISSE_ToolCallDeltaEvents(t *testing.T) {
	stream := `data: {"id":"r7","model":"qwen3","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}

data: {"id":"r7","model":"qwen3","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_tool1","type":"function","function":{"name":"list_files","arguments":""}}]},"finish_reason":null}]}

data: {"id":"r7","model":"qwen3","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"dir\":"}}]},"finish_reason":null}]}

data: {"id":"r7","model":"qwen3","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"/tmp\"}"}}]},"finish_reason":null}]}

data: {"id":"r7","model":"qwen3","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]
`
	events := collectSSE(sseBody(stream))

	var toolCallStart *cif.CIFContentDelta
	var argDeltas []string
	for _, e := range events {
		delta, ok := e.(cif.CIFContentDelta)
		if !ok {
			continue
		}
		if _, isToolArgs := delta.Delta.(cif.ToolArgumentsDelta); isToolArgs {
			if delta.ContentBlock != nil {
				toolCallStart = &delta
			} else {
				argDeltas = append(argDeltas, delta.Delta.(cif.ToolArgumentsDelta).PartialJSON)
			}
		}
	}

	if toolCallStart == nil {
		t.Fatal("expected a tool call block-start delta, got none")
	}
	tc, ok := toolCallStart.ContentBlock.(cif.CIFToolCallPart)
	if !ok {
		t.Fatalf("expected CIFToolCallPart ContentBlock, got %T", toolCallStart.ContentBlock)
	}
	if tc.ToolName != "list_files" {
		t.Errorf("expected ToolName=list_files, got %q", tc.ToolName)
	}
	if tc.ToolCallID != "call_tool1" {
		t.Errorf("expected ToolCallID=call_tool1, got %q", tc.ToolCallID)
	}

	combined := strings.Join(argDeltas, "")
	if !strings.Contains(combined, "/tmp") {
		t.Errorf("expected accumulated args to contain /tmp, got %q", combined)
	}
}

// TestParseOpenAISSE_InterleavedMultiToolCalls verifies that when a provider
// streams multiple tool calls interleaved by provider-side tool_call.index,
// continuation argument deltas are attached to the correct original tool block.
// This matches DashScope/Qwen behavior and guards against using the latest local
// contentBlockIndex for all continuation chunks.
func TestParseOpenAISSE_InterleavedMultiToolCalls(t *testing.T) {
	stream := `data: {"id":"r9","model":"qwen3","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}

data: {"id":"r9","model":"qwen3","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"first_tool","arguments":""}}]},"finish_reason":null}]}

data: {"id":"r9","model":"qwen3","choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call_b","type":"function","function":{"name":"second_tool","arguments":""}}]},"finish_reason":null}]}

data: {"id":"r9","model":"qwen3","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":\"/tmp/a\"}"}}]},"finish_reason":null}]}

data: {"id":"r9","model":"qwen3","choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"function":{"arguments":"{\"query\":\"hello\"}"}}]},"finish_reason":null}]}

data: {"id":"r9","model":"qwen3","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]
`
	events := collectSSE(sseBody(stream))

	ch := make(chan cif.CIFStreamEvent, len(events))
	for _, e := range events {
		ch <- e
	}
	close(ch)

	resp, err := CollectStream(ch)
	if err != nil {
		t.Fatalf("CollectStream returned error: %v", err)
	}
	if resp.StopReason != cif.StopReasonToolUse {
		t.Fatalf("stop reason = %q, want %q", resp.StopReason, cif.StopReasonToolUse)
	}

	if len(resp.Content) != 2 {
		t.Fatalf("expected 2 tool calls in response content, got %d: %#v", len(resp.Content), resp.Content)
	}

	first, ok := resp.Content[0].(cif.CIFToolCallPart)
	if !ok {
		t.Fatalf("expected first content part to be CIFToolCallPart, got %T", resp.Content[0])
	}
	second, ok := resp.Content[1].(cif.CIFToolCallPart)
	if !ok {
		t.Fatalf("expected second content part to be CIFToolCallPart, got %T", resp.Content[1])
	}

	if first.ToolCallID != "call_a" || first.ToolName != "first_tool" {
		t.Fatalf("unexpected first tool call: %#v", first)
	}
	if second.ToolCallID != "call_b" || second.ToolName != "second_tool" {
		t.Fatalf("unexpected second tool call: %#v", second)
	}

	if got := first.ToolArguments["path"]; got != "/tmp/a" {
		t.Fatalf("first tool args path = %#v, want /tmp/a", got)
	}
	if got := second.ToolArguments["query"]; got != "hello" {
		t.Fatalf("second tool args query = %#v, want hello", got)
	}
}

// TestParseOpenAISSE_ToolCallSingleChunkWithArguments verifies that when a provider
// (e.g. GLM/Zhipu) sends the tool call id, name, and complete arguments all in a
// single SSE chunk (rather than streaming arguments separately), the arguments are
// not lost. This is the root cause of the "missing required parameter" errors seen
// when glm-5.1 via the Alibaba provider calls tools like Agent or Glob.
func TestParseOpenAISSE_ToolCallSingleChunkWithArguments(t *testing.T) {
	stream := `data: {"id":"r8","model":"glm-5.1","choices":[{"index":0,"delta":{"role":"assistant","content":null},"finish_reason":null}]}

data: {"id":"r8","model":"glm-5.1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"tool-abc123","type":"function","function":{"name":"Glob","arguments":"{\"pattern\":\"**/*.go\"}"}}]},"finish_reason":null}]}

data: {"id":"r8","model":"glm-5.1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

data: [DONE]
`
	events := collectSSE(sseBody(stream))

	var toolCallStart *cif.CIFContentDelta
	var argDeltas []string
	for _, e := range events {
		delta, ok := e.(cif.CIFContentDelta)
		if !ok {
			continue
		}
		if _, isToolArgs := delta.Delta.(cif.ToolArgumentsDelta); isToolArgs {
			if delta.ContentBlock != nil {
				toolCallStart = &delta
			} else {
				argDeltas = append(argDeltas, delta.Delta.(cif.ToolArgumentsDelta).PartialJSON)
			}
		}
	}

	if toolCallStart == nil {
		t.Fatal("expected a tool call block-start delta, got none")
	}
	tc, ok := toolCallStart.ContentBlock.(cif.CIFToolCallPart)
	if !ok {
		t.Fatalf("expected CIFToolCallPart ContentBlock, got %T", toolCallStart.ContentBlock)
	}
	if tc.ToolName != "Glob" {
		t.Errorf("expected ToolName=Glob, got %q", tc.ToolName)
	}
	if tc.ToolCallID != "tool-abc123" {
		t.Errorf("expected ToolCallID=tool-abc123, got %q", tc.ToolCallID)
	}

	combined := strings.Join(argDeltas, "")
	if combined != `{"pattern":"**/*.go"}` {
		t.Errorf("expected arguments to be preserved, got %q", combined)
	}

	// Verify stop reason is correctly set to ToolUse
	var streamEnd *cif.CIFStreamEnd
	for _, e := range events {
		if end, ok := e.(cif.CIFStreamEnd); ok {
			streamEnd = &end
		}
	}
	if streamEnd == nil || streamEnd.StopReason != cif.StopReasonToolUse {
		t.Errorf("expected StopReasonToolUse, got %v", streamEnd)
	}
}

// ─── CIFMessagesToOpenAI: CIFThinkingPart ────────────────────────────────────

// TestCIFMessagesToOpenAIThinkingPartForwardsAsReasoningContent verifies that a
// CIFThinkingPart in an assistant message is forwarded as reasoning_content so
// DashScope Qwen models can use the prior thinking in multi-turn conversations.
func TestCIFMessagesToOpenAIThinkingPartForwardsAsReasoningContent(t *testing.T) {
	messages := []cif.CIFMessage{
		cif.CIFAssistantMessage{
			Role: "assistant",
			Content: []cif.CIFContentPart{
				cif.CIFThinkingPart{Type: "thinking", Thinking: "let me reason..."},
				cif.CIFTextPart{Type: "text", Text: "The answer is 42."},
			},
		},
	}

	result := CIFMessagesToOpenAI(messages)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	msg := result[0]
	if msg["role"] != "assistant" {
		t.Errorf("expected role=assistant, got %v", msg["role"])
	}
	if msg["content"] != "The answer is 42." {
		t.Errorf("expected content text, got %v", msg["content"])
	}
	if rc, ok := msg["reasoning_content"]; !ok || rc != "let me reason..." {
		t.Errorf("expected reasoning_content='let me reason...', got %v (present=%v)", rc, ok)
	}
}

// TestCIFMessagesToOpenAINoThinkingNoReasoningContent verifies that reasoning_content
// is absent from messages that have no CIFThinkingPart.
func TestCIFMessagesToOpenAINoThinkingNoReasoningContent(t *testing.T) {
	messages := []cif.CIFMessage{
		cif.CIFAssistantMessage{
			Role: "assistant",
			Content: []cif.CIFContentPart{
				cif.CIFTextPart{Type: "text", Text: "Hello."},
			},
		},
	}

	result := CIFMessagesToOpenAI(messages)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if _, ok := result[0]["reasoning_content"]; ok {
		t.Errorf("expected reasoning_content to be absent for non-thinking message")
	}
}

// ─── CollectStream: ThinkingDelta preservation ───────────────────────────────

// TestCollectStream_ThinkingDeltaPreserved verifies that ThinkingDelta events are
// accumulated by CollectStream and appear as a CIFThinkingPart in the response,
// placed before any text content (matching Anthropic ordering).
func TestCollectStream_ThinkingDeltaPreserved(t *testing.T) {
	ch := make(chan cif.CIFStreamEvent, 10)
	ch <- cif.CIFStreamStart{Type: "stream_start", ID: "test-id", Model: "qwen3.6-plus"}
	ch <- cif.CIFContentDelta{
		Type:         "content_delta",
		Index:        -1,
		ContentBlock: cif.CIFThinkingPart{Type: "thinking", Thinking: ""},
		Delta:        cif.ThinkingDelta{Type: "thinking_delta", Thinking: "Let me think..."},
	}
	ch <- cif.CIFContentDelta{
		Type:  "content_delta",
		Index: -1,
		Delta: cif.ThinkingDelta{Type: "thinking_delta", Thinking: " about this problem."},
	}
	ch <- cif.CIFContentDelta{
		Type:         "content_delta",
		Index:        0,
		ContentBlock: cif.CIFTextPart{Type: "text", Text: ""},
		Delta:        cif.TextDelta{Type: "text_delta", Text: "The answer is 42."},
	}
	ch <- cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonEndTurn}
	close(ch)

	resp, err := CollectStream(ch)
	if err != nil {
		t.Fatalf("CollectStream returned error: %v", err)
	}

	if len(resp.Content) != 2 {
		t.Fatalf("expected 2 content parts (thinking + text), got %d: %v", len(resp.Content), resp.Content)
	}

	// Thinking must come first.
	thinking, ok := resp.Content[0].(cif.CIFThinkingPart)
	if !ok {
		t.Fatalf("expected first content part to be CIFThinkingPart, got %T", resp.Content[0])
	}
	if thinking.Thinking != "Let me think... about this problem." {
		t.Errorf("unexpected thinking content: %q", thinking.Thinking)
	}

	// Text must follow.
	text, ok := resp.Content[1].(cif.CIFTextPart)
	if !ok {
		t.Fatalf("expected second content part to be CIFTextPart, got %T", resp.Content[1])
	}
	if text.Text != "The answer is 42." {
		t.Errorf("unexpected text content: %q", text.Text)
	}
}

// TestCollectStream_NoThinkingWhenAbsent verifies that when no ThinkingDelta events
// arrive, the response contains no CIFThinkingPart.
func TestCollectStream_PreservesIndexedOrder(t *testing.T) {
	ch := make(chan cif.CIFStreamEvent, 10)
	sig := "sig-ordered"
	ch <- cif.CIFStreamStart{Type: "stream_start", ID: "ordered", Model: "qwen3.6-plus"}
	ch <- cif.CIFContentDelta{
		Type:         "content_delta",
		Index:        1,
		ContentBlock: cif.CIFToolCallPart{Type: "tool_call", ToolCallID: "call_1", ToolName: "Glob", ToolArguments: map[string]interface{}{}},
		Delta:        cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `{"pattern":"**/*.go"}`},
	}
	ch <- cif.CIFContentDelta{
		Type:         "content_delta",
		Index:        -1,
		ContentBlock: cif.CIFThinkingPart{Type: "thinking", Thinking: "", Signature: &sig},
		Delta:        cif.ThinkingDelta{Type: "thinking_delta", Thinking: "think first"},
	}
	ch <- cif.CIFContentDelta{
		Type:         "content_delta",
		Index:        0,
		ContentBlock: cif.CIFTextPart{Type: "text", Text: ""},
		Delta:        cif.TextDelta{Type: "text_delta", Text: "then text"},
	}
	ch <- cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonToolUse}
	close(ch)

	resp, err := CollectStream(ch)
	if err != nil {
		t.Fatalf("CollectStream returned error: %v", err)
	}
	if len(resp.Content) != 3 {
		t.Fatalf("expected 3 content parts, got %d: %#v", len(resp.Content), resp.Content)
	}
	if _, ok := resp.Content[0].(cif.CIFThinkingPart); !ok {
		t.Fatalf("expected first content part to be thinking, got %T", resp.Content[0])
	}
	if _, ok := resp.Content[1].(cif.CIFTextPart); !ok {
		t.Fatalf("expected second content part to be text, got %T", resp.Content[1])
	}
	tool, ok := resp.Content[2].(cif.CIFToolCallPart)
	if !ok {
		t.Fatalf("expected third content part to be tool call, got %T", resp.Content[2])
	}
	if tool.ToolName != "Glob" {
		t.Fatalf("expected tool name Glob, got %q", tool.ToolName)
	}
}

func TestCollectStream_NoThinkingWhenAbsent(t *testing.T) {
	ch := make(chan cif.CIFStreamEvent, 5)
	ch <- cif.CIFStreamStart{Type: "stream_start", ID: "test-id-2", Model: "qwen3-max"}
	ch <- cif.CIFContentDelta{
		Type:         "content_delta",
		Index:        0,
		ContentBlock: cif.CIFTextPart{Type: "text", Text: ""},
		Delta:        cif.TextDelta{Type: "text_delta", Text: "Hello."},
	}
	ch <- cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonEndTurn}
	close(ch)

	resp, err := CollectStream(ch)
	if err != nil {
		t.Fatalf("CollectStream returned error: %v", err)
	}

	for _, part := range resp.Content {
		if _, isThinking := part.(cif.CIFThinkingPart); isThinking {
			t.Error("expected no CIFThinkingPart when no ThinkingDelta events were emitted")
		}
	}
}
