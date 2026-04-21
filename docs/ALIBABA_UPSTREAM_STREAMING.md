# Alibaba DashScope upstream behavior

## Current rule

For the Alibaba provider, OmniLLM **does not use upstream SSE / streaming**.

- Client requests may still use `stream=true`
- OmniLLM executes the upstream DashScope request as **non-streaming**
- OmniLLM then re-streams the completed CIF response locally back to the client

## Why

DashScope's streaming chat-completions endpoint has proven unreliable for OmniLLM's request shapes and can reject otherwise valid requests with HTTP 400 before any SSE data is emitted, for example:

- `Required body invalid, please check the request body format.`

This was especially visible with Anthropic-style `/v1/messages` requests routed to Alibaba models such as `qwen3.6-plus`.

## Implementation

The Alibaba adapter's streaming entrypoint buffers upstream non-streaming responses instead of calling the upstream SSE endpoint:

- `internal/providers/alibaba/adapter_models.go`
  - `ExecuteStream()` calls `Execute()`
  - then returns `shared.StreamResponse(response)`

## Effect

This keeps the external API behavior stable for clients while avoiding upstream DashScope SSE failures.

- **Client-facing streaming:** preserved
- **Upstream Alibaba streaming:** disabled
- **Tool use / Anthropic compatibility:** preserved through CIF conversion and local re-streaming
