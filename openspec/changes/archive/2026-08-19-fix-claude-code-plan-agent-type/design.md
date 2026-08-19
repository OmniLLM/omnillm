## Context

Claude Code uses the Anthropic Messages API and supplies an `Agent` tool whose `subagent_type` selects a registered agent. In Plan mode, Claude Code 2.1.235 can place an internal `scriptPath` diagnostic immediately after the `Plan` label in model-visible context. A non-Claude model can reproduce the combined text as the argument value:

`Plan-scriptPath not found: plan file is managed by plan mode system, use Plan agent output only.`

The tool call remains syntactically valid and reaches Claude Code, but the client performs an exact agent-name lookup and rejects it. The failed result is added to the conversation, so an unconstrained retry can repeat the same value. OmniLLM currently preserves the non-empty string as required by the general tool-call fidelity contract.

This change touches the CIF translation boundary, the public Anthropic Messages response path, streaming, response caching, and client compatibility, so it requires an explicit cross-capability design.

## Goals / Non-Goals

**Goals:**

- Allow Claude Code to execute the intended `Plan` agent when a model emits the exact known Plan-mode sentinel.
- Require request and schema evidence before changing a model-emitted argument.
- Produce equivalent behavior for non-streaming, streaming, and cached responses.
- Keep the repair isolated from other clients, tools, fields, and malformed values.
- Record enough metadata to identify repair activity without logging argument or prompt content.

**Non-Goals:**

- Correct arbitrary invalid agent names or fuzzy-match arbitrary enum values.
- Parse general Claude Code diagnostics or depend on human-readable error text beyond the one known sentinel.
- Change Claude Code's Plan-mode implementation or suppress client tool errors.
- Apply the repair to Chat Completions, Responses, or callers that do not identify as Claude Code.
- Change provider selection based on whether the upstream model is Claude-family or non-Claude.

## Decisions

### Gate the repair by the Claude Code Messages request

Enable the compatibility rule only on `/v1/messages` when the request `User-Agent` identifies Claude Code using its `claude-cli/<version>` product token. Thread an explicit compatibility policy through response normalization instead of making the shared normalizer infer an HTTP client.

The repair is useful primarily for non-Claude models, but provider and model names are configurable and aliases can hide model families. Client, route, tool, schema, and exact-value evidence form a deterministic boundary without brittle model-name classification.

Alternative considered: enable the rule for all Anthropic-compatible callers. Rejected because another client could legitimately use the same string and should retain normal tool-argument fidelity.

Alternative considered: restrict by a list of non-Claude model names. Rejected because aliases, future models, and provider-qualified routes make the list incomplete and operationally fragile.

### Match one exact sentinel and one exact argument location

Inspect only a completed call to the tool named `Agent`. Change only the top-level string property `subagent_type` when its complete value exactly equals:

`Plan-scriptPath not found: plan file is managed by plan mode system, use Plan agent output only.`

Do not use prefix stripping, substring replacement, regular-expression extraction, or case folding. Preserve calls with suffixes, prefixes, alternate diagnostics, non-string values, or malformed JSON.

Alternative considered: split any value at `-scriptPath not found:`. Rejected because it would turn unrecognized model output into executable client instructions using a broad textual heuristic.

### Require the declared schema to authorize `Plan`

Resolve the request's exact `Agent` tool definition and inspect the directly declared `subagent_type` property. Apply the repair only when that property schema explicitly allows the string `Plan`, such as through a direct `const` or string `enum`. If the tool, property, schema, or allowed value cannot be established, preserve the model output unchanged.

This makes the request's own tool contract the authority for the repaired value. It also prevents OmniLLM from inventing an agent type for an older, newer, or unrelated `Agent` tool schema.

Alternative considered: always replace the known sentinel with `Plan`. Rejected because tool names are not globally reserved and schemas can vary by Claude Code version or plugin configuration.

### Normalize before client emission and cache accumulation

Apply the compatibility policy to completed canonical tool arguments before Anthropic serialization. For streams, continue buffering tool-argument fragments by call index, then apply both existing optional-empty normalization and the new compatibility repair to the completed object before emission. Cache accumulation therefore observes the repaired canonical stream, and replay emits the same value without needing the original HTTP user agent.

Cached entries created before this change are not retroactively rewritten. A cache hit for an affected request must nevertheless satisfy the same client-facing contract, so the Anthropic cache replay path will apply the compatibility repair when sufficient cached tool-call and current request schema context are available, or bypass an unsafe hit if that context cannot produce contract-equivalent output.

### Emit metadata-only repair diagnostics

When a repair is applied, emit one structured diagnostic per repaired tool call containing request ID, API shape, model/provider when known, tool name, tool-call ID, stream/cache state, and a stable repair reason. Do not log the original or repaired argument object, prompt content, or result content.

## Risks / Trade-offs

- **[Risk] Claude Code changes the diagnostic text.** The exact match stops applying and the value is preserved. Add a new reviewed compatibility case rather than broadening the matcher implicitly.
- **[Risk] Tool schemas express allowed values through complex JSON Schema composition.** Initially accept only directly verifiable `const` or string `enum` evidence and preserve on uncertainty.
- **[Risk] Buffering affects incremental tool-argument delivery.** The existing normalizer already buffers arguments for schema-aware normalization; retain independent call indexes and existing flush boundaries.
- **[Risk] Cache entries may predate the compatibility policy.** Test cache hits explicitly and bypass a hit rather than emit a known-invalid call when safe repair context is unavailable.
- **[Risk] User-Agent matching could include lookalike strings.** Parse the product token instead of using an unrestricted substring match.

## Migration Plan

No persisted-data or configuration migration is required. Deploy as a backward-compatible Claude Code response repair. Existing cache data remains readable; affected legacy entries are repaired when safe or bypassed. Rollback consists of removing the policy and its diagnostics, restoring byte-for-byte preservation of the sentinel.
