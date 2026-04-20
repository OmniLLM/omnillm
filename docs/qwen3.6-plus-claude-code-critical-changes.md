# Qwen3.6-plus + Claude Code: Critical Changes Record

## Context

Claude Code sends requests to omnillm via the Anthropic `/v1/messages` API (always streaming). When the upstream provider is Qwen3.6-plus on Alibaba DashScope, the request must flow: Anthropic format → CIF → OpenAI-compatible format → DashScope → response back through CIF → Anthropic format.

Prior to commit `dfa3cec5e2c574707d1a2c9b5d8873195ff58bcf`, this path was broken. The changes below (spanning commits `49f2351` through `69f1619`) fixed it.

## Critical Fixes

### 1. Alibaba provider refactored onto `openaicompat` shared layer

**Commits:** `49f2351`, `105a2bf`

**Before:** The Alibaba provider had ~744 lines of bespoke HTTP handling, serialization, and SSE parsing. It supported two modes: `openai-compatible` (chat completions) and `anthropic` (DashScope `/apps/anthropic/v1` endpoint). The Anthropic-mode path was unreliable and buggy.

**After:** The Anthropic-mode path was removed entirely. Alibaba is now a thin configuration layer on `internal/providers/openaicompat`. The `Adapter` builds requests via `openaicompat.BuildChatRequest()` and sends to DashScope's `/compatible-mode/v1/chat/completions` endpoint. All request/response serialization uses the shared `openaicompat` package's typed structs (`ChatRequest`, `ChatResponse`, `StreamChunk`, etc.) rather than raw `map[string]interface{}`.

**Why critical:** Without this, the Alibaba provider couldn't reliably translate CIF requests to DashScope's OpenAI-compatible format or parse the responses back. The old Anthropic-mode endpoint (`/apps/anthropic/v1`) was a separate incompatible protocol that caused constant breakage.

---

### 2. Tool call ID alias handling (`call_id` vs `id`)

**Commit:** `49f2351`

**Problem:** DashScope (and some other OpenAI-compatible providers) send `call_id` instead of `id` in tool call deltas. The ingestion code only looked at `id`, so tool call IDs were sometimes empty — breaking the tool loop since Claude Code requires matching `tool_use_id` on `tool_result` messages.

**Fix:** 
- `ToolCall` struct now has both `ID` (`json:"id,omitempty"`) and `CallID` (`json:"call_id,omitempty"`) fields.
- Ingestion uses `firstNonEmpty(toolCall.ID, toolCall.CallID)` to pick whichever is populated.
- Anthropic `tool_result` blocks fall back to `block.ID` when `block.ToolUseID` is empty.
- Outbound serialization emits both `"id"` and `"call_id"` in OpenAI-format tool calls via `shared/openai.go`.

**Why critical:** Without matching tool call IDs, Claude Code's tool loop breaks completely — it can't correlate `tool_use` blocks with `tool_result` responses.

---

### 3. Thinking block suppression for non-opted clients

**Commits:** `49f2351`, `69f1619`

**Problem:** Qwen3.6-plus with `enable_thinking: true` returns `reasoning_content` in responses. The `openaicompat` SSE parser converts this to `CIFThinkingPart`. But Claude Code (standard, without `anthropic-beta: interleaved-thinking`) **cannot parse thinking blocks** and silently stops processing the stream. This means any `tool_use` blocks that follow a thinking block are dropped — the tool loop appears to hang.

**Fix:** `SerializeToAnthropicWithSuppression()` suppresses thinking blocks when `suppressThinking` is true. The route handler sets this flag based on whether the client sent `anthropic-beta: interleaved-thinking`:
```go
suppressThinking := !strings.Contains(c.GetHeader("anthropic-beta"), "interleaved-thinking")
```

The streaming path (`handleAnthropicStreamingResponse`) also sets `state.SuppressThinkingBlocks = true` when the beta header is absent.

**Why critical:** This is the single most impactful fix. Without it, Claude Code would silently discard every tool_use response from Qwen3.6-plus, making the tool loop completely non-functional. The model would appear to "stop responding" after its first thinking-enabled turn.

---

### 4. `enable_thinking` suppressed when tools are present

**Commit:** `49f2351`

**Problem:** DashScope rejects requests that combine `enable_thinking: true` with tools. When Claude Code sends a tool-use request to Qwen3.6-plus, the request would fail with a DashScope API error.

**Fix:** The Alibaba adapter's `buildRequest` now only injects `enable_thinking: true` when `len(request.Tools) == 0`:
```go
if IsReasoningModel(model) && len(request.Tools) == 0 {
    extras["enable_thinking"] = true
}
```

**Why critical:** Without this, every Claude Code request that includes tools (which is most requests during a tool loop) would be rejected by DashScope.

---

### 5. Tool-call-aware `finish_reason` upgrade in SSE parser

**Commit:** `49f2351` (new `openaicompat/serialization.go`)

**Problem:** DashScope sometimes sends `finish_reason: "stop"` when tool calls were present in the stream. Without correction, this would be mapped to `end_turn` in CIF, then serialized as `stop_reason: "end_turn"` to Claude Code. Claude Code would not recognize that the model wants to invoke a tool, breaking the tool loop.

**Fix:** The `ParseSSE` function in `openaicompat/serialization.go` tracks `toolCallsSeen`. When the stream ends with `finish_reason: "stop"` but tool calls were observed, the stop reason is upgraded to `StopReasonToolUse`:
```go
if stopReason != cif.StopReasonToolUse && len(toolCallsSeen) > 0 {
    stopReason = cif.StopReasonToolUse
}
```

**Why critical:** If Claude Code receives `stop_reason: "end_turn"` instead of `"tool_use"`, it treats the response as a final answer rather than a tool invocation, breaking the agentic loop.

---

### 6. Streaming fallback to non-streaming

**Commit:** `105a2bf`, refined in `69f1619`

**Problem:** If the SSE stream from DashScope fails before any data is sent (connection error, timeout), the entire request fails with no response to Claude Code.

**Fix:** `handleAnthropicStreamingResponse` checks `shouldFallbackToNonStreaming(err)` and `allowStreamingFallback(canonicalRequest)`. If streaming fails before stream start, it retries as a non-streaming request:
```go
if shouldFallbackToNonStreaming(err) && allowStreamingFallback(canonicalRequest) {
    canonicalRequest.Stream = false
    return handleAnthropicNonStreamingResponse(...)
}
```

**Why critical:** Improves reliability — a transient DashScope streaming failure doesn't kill the entire request.

---

## Commit Range

```
49f2351 feat: extract openaicompat provider, refactor Alibaba, and fix tool call ID aliases
b363cc4 feat: add openai-compatible provider type
d80d860 feat: frontend support for openai-compatible provider with user-defined models
a635eab feat: allow model IDs to be entered during openai-compatible provider setup
105a2bf feat: expand openai-compatible provider with responses API, streaming, and admin enhancements
86225a0 feat: add ToolConfig page for editing Claude Code and Codex configs
69f1619 chore: update providers and routes with openai-compatible improvements
fb18e69 fix: reduce tool loop log verbosity from info to debug
```

Base commit: `dfa3cec5e2c574707d1a2c9b5d8873195ff58bcf`

## Key Files Changed

| File | Role |
|------|------|
| `internal/providers/alibaba/provider.go` | Refactored to thin wrapper on `openaicompat` |
| `internal/providers/openaicompat/serialization.go` | New: CIF↔OpenAI request/response/stream serialization |
| `internal/providers/openaicompat/types.go` | New: typed structs for OpenAI-compatible wire format |
| `internal/providers/openaicompat/http.go` | New: Execute/Stream HTTP call helpers |
| `internal/providers/shared/openai.go` | Added `call_id` alias, `NormalizeOpenAICompatibleAPIFormat` |
| `internal/ingestion/ingestion.go` | `ToolCall.CallID` alias, `firstNonEmpty`, `tool_result.ID` fallback |
| `internal/serialization/serialization.go` | `SerializeToAnthropicWithSuppression` for thinking block control |
| `internal/routes/messages.go` | Thinking suppression check, streaming fallback helper |
