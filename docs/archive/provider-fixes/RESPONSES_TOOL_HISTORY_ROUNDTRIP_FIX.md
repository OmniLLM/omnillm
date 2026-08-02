# Responses Tool History Round-Trip Fix

## Context

OpenAI-style `/v1/responses` requests that reused prior tool-call history could fail after CIF conversion when the prior assistant turn contained:

- an assistant `message` item, and/or
- multiple sibling `function_call` items,
- followed by matching `function_call_output` items.

This was reproduced live against `gpt-5-mini` via GitHub Copilot with upstream errors stating that assistant `tool_calls` were not followed by matching tool responses.

## What Changed

### Before

`internal/ingestion/from_responses.go` translated each Responses input item into its own CIF message:

- each assistant `message` became a standalone `CIFAssistantMessage`
- each `function_call` became another standalone `CIFAssistantMessage`
- each `function_call_output` became a `CIFUserMessage`

That broke assistant-turn grouping during `/v1/responses -> CIF -> chat.completions` flows, because multiple tool calls from one logical assistant turn were split across multiple assistant messages.

### After

`internal/ingestion/from_responses.go` now merges contiguous assistant-side Responses items into a single CIF assistant turn:

- assistant `message` content is accumulated into a pending assistant turn
- sibling `function_call` items append `CIFToolCallPart` values onto the same assistant turn
- `function_call_output`, `user`, `system`, and `developer` items flush the pending assistant turn before adding their own CIF messages

This preserves tool-call adjacency for downstream chat-completions providers while keeping Azure Responses flows valid.

## Why This Is Critical

This is a protocol-level bug fix affecting live tool loops on public API routes:

- `/v1/responses` could return `502` even when the original tool history was valid
- GitHub Copilot `gpt-5-mini` rejected the malformed downstream chat-completions history
- Alibaba `qwen3.6-plus` and other chat-completions upstreams depended on the same turn structure when reached through CIF

Without this fix, Responses API clients could not reliably continue conversations after a prior tool-using assistant turn.

## Affected Files

- `internal/ingestion/from_responses.go`
- `internal/ingestion/responses_test.go`

## Verification

- Added regression coverage for merged assistant `function_call` history in `internal/ingestion/responses_test.go`
- Verified `go test ./internal/ingestion`
- Live-verified after rebuild:
  - `/v1/responses` -> `gpt-5-mini` -> `200`
  - `/v1/responses` -> `gpt-5.4` -> `200`
  - `/v1/responses` -> `qwen3.6-plus` -> `200`
  - `/v1/chat/completions` -> `gpt-5-mini` -> `200`
  - `/v1/chat/completions` -> `gpt-5.4` -> `200`
  - `/v1/chat/completions` -> `qwen3.6-plus` -> `200`

## Commit Range

- Working tree changes after current `HEAD` (uncommitted fix set)
