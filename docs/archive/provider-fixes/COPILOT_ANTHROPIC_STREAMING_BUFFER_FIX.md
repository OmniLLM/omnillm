# Copilot Anthropic Streaming Buffer Fix

## Context

GitHub Copilot models can expose different upstream behavior across
chat-completions and Responses API routes. Anthropic-shape streaming requests
need to serialize back to Anthropic SSE, but Copilot's native upstream streaming
path can produce events that are not reliably compatible with that downstream
shape for GPT-5-family models.

## What Changed

### Before

`internal/providers/copilot/adapter.go` streamed Anthropic-originated requests
directly to the selected Copilot upstream streaming endpoint:

- GPT-5-family models used `/responses` streaming.
- Other models used `/chat/completions` streaming.
- Downstream Anthropic SSE conversion depended on the upstream stream shape.

### After

Copilot now detects requests whose CIF extension records
`InboundAPIShape == "anthropic"`. For those streaming requests, the adapter:

- executes the upstream request through the non-streaming `Execute` path,
- preserves the normal Copilot endpoint selection and auth retry behavior,
- replays the completed CIF response as a synthetic CIF stream with
  `shared.StreamResponse`.

OpenAI-shape streaming requests continue to use upstream streaming directly.

## Why This Is Critical

This is a provider protocol change for public Anthropic-compatible streaming
routes. Without buffering, Anthropic clients can receive incompatible or
incomplete SSE behavior when routed through GitHub Copilot, especially for
models that require the Responses API upstream.

The fix trades first-token latency for protocol correctness only on the
Anthropic inbound shape; OpenAI and Responses inbound shapes retain upstream
streaming behavior.

## Affected Files

- `internal/providers/copilot/adapter.go`
- `internal/providers/copilot/copilot_test.go`
- `internal/providers/shared/response_stream.go`

## Verification

- Added Copilot regression coverage for Anthropic inbound streaming buffering.
- Added Copilot regression coverage that OpenAI-shape GPT-5 streaming still uses
  upstream `/responses` streaming.
- Verified `go test ./internal/providers/copilot ./internal/providers/shared ./internal/serialization`
- Verified `bun run typecheck`
- Verified `bun run build`

## Commit Range

- `1fc5ac12047c0b94869ed3d95264b7207840ed80..2391a4f5d297ec5d12061fef4487ce4c1fea8fc4`
