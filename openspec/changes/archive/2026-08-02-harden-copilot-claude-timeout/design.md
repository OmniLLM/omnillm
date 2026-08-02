## Context

Copilot currently has three shared clients: a 120-second ordinary request client, a 300-second Responses client, and an unbounded streaming client. Native Claude streaming is required for tool-call fidelity, but a failure to receive initial headers is currently observed at the 120-second ordinary budget. The seven correlated failures all used the same Copilot provider/model and ended as 502 at 120024-120029 ms.

## Goals / Non-Goals

**Goals:**
- Allow Copilot Claude reasoning requests more time to emit initial headers.
- Keep ordinary operations bounded at their existing timeout.
- Preserve native Claude SSE processing after headers.
- Make future timeouts directly correlatable to request and configuration.

**Non-Goals:**
- Retrying an exhausted request against the same Copilot instance.
- Changing provider failover ordering or HTTP status mapping.
- Moving Claude models to the Responses endpoint.
- Logging prompts, credentials, or response bodies.

## Decisions

### Add a dedicated Claude header client

Create a Copilot Claude client using the shared production transport and a default 300-second budget, configurable through `COPILOT_CLAUDE_TIMEOUT`. Select it only for Claude chat-completions. This reuses the already-tested reasoning budget without widening the ordinary 120-second timeout for model discovery, embeddings, and faster chat models.

Using the unbounded stream client for the entire request was considered, but it provides no upper bound when the upstream never returns headers. Raising the global default was rejected because it would delay failure for unrelated operations.

### Separate header acquisition from body streaming

The selected client budget applies only until `Do` returns response headers. Once a streaming response is established, the response body is consumed by the existing SSE parser and remains governed by caller cancellation and transport health rather than a fixed total request deadline. This avoids severing long valid generations.

### Log timeout context at the owning adapter

On timeout, emit a structured warning before wrapping and returning the error. Include provider instance, request ID obtained from canonical incoming headers when present, canonical model, endpoint name, budget, and elapsed milliseconds. Route-level logging remains the final error record, but the owning provider log becomes actionable.

## Risks / Trade-offs

- **A genuinely stalled Claude request now fails later** → Keep a finite five-minute default and an operator override.
- **Concurrent long waits consume more connections** → Existing provider and gateway concurrency bounds remain in force; only Claude uses the expanded budget.
- **Request ID may be absent in canonical headers** → Log an empty/omitted identifier while retaining provider/model/endpoint correlation.
- **Client timeout covers body for accidental non-streaming Claude calls** → Use the same larger budget there because Anthropic buffering and failover can invoke non-streaming execution; native streams remain body-unbounded.

## Migration Plan

Deploy with the default 300-second Claude timeout. Operators needing a different bound may set `COPILOT_CLAUDE_TIMEOUT` using a Go duration or seconds. Rollback removes the dedicated client selection and returns Claude requests to the 120-second ordinary budget.
