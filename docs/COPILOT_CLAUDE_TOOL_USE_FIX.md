# Copilot Claude Tool Use Fix

## Context

Claude-family models routed through GitHub Copilot can return OpenAI-compatible tool calls with provider-specific details that do not map cleanly to Anthropic clients. Claude Code reported that some Claude tool calls could not be parsed when using Anthropic `/v1/messages` streaming through OmniLLM.

## What Changed

Before:

- Copilot chat-completions streaming only accepted `id`, did not preserve provider `tool_calls[].index` for argument continuations, and could drop arguments emitted in the first tool-call chunk.
- Copilot non-streaming responses only accepted `id`, not `call_id`.
- Anthropic serialization forwarded Copilot tool ids such as `tooluse_...` unchanged.

After:

- Copilot streaming accepts `id` and `call_id`, maps argument deltas by provider index, preserves same-chunk arguments, and upgrades `[DONE]` or `stop` endings to tool use when tool calls were observed.
- Copilot non-streaming accepts `call_id` and upgrades malformed `stop` responses to tool use when tool calls are present.
- Anthropic responses normalize outbound tool-use ids to Anthropic-style `toolu_...` ids.

## Why It Is Critical

Claude Code depends on Anthropic-compatible tool-use SSE blocks. If the tool id or argument stream is malformed, the local client cannot execute tools, so agent workflows fail immediately after the model requests tool use.

## Affected Files

- `internal/providers/copilot/sse_parser.go`
- `internal/providers/copilot/payload.go`
- `internal/providers/copilot/copilot_test.go`
- `internal/serialization/to_anthropic.go`
- `internal/serialization/serialization_test.go`
- `internal/serialization/serialization_master_test.go`

## Validation

- `go test ./internal/serialization`
- `go test ./internal/providers/copilot`
- `go test ./internal/ingestion ./internal/serialization ./internal/providers/shared ./internal/providers/openaicompat ./internal/providers/copilot ./internal/server`
- `bun test tests/claude-code-qwen-tooluse.test.ts`
- Live Claude Agent SDK checks against patched backend on port 5100:
  - `claude-haiku-4.5` emitted and executed `Read`
  - `claude-sonnet-4.6` emitted and executed `Read`

## Commit Range

Pending commit.
