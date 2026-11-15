## Context

The provider serializes all function schemas into Gemini `parameters` using `shared.SanitizeGeminiSchema`. Google's documented `parameters` schema is a restricted OpenAPI representation with a required type; `parametersJsonSchema` is a mutually exclusive arbitrary JSON Schema object. Claude Code's SendMessage schema contains an untyped nested `anyOf` container and discriminator `const` fields. The original, normalized, and sanitized versions all pass a local draft-2020-12 schema check, but the upstream restricted representation rejects this tool.

The schema-only diagnostic captured 41 definitions. A full captured-set request through the installed gateway reproduced HTTP 502; the equivalent direct restricted-envelope request identified `SendMessage` at index 18. Historical interactive-session index 21 cannot be mapped solely by this fresh capture, because tool order differs by client mode and loaded tools. A controlled one-tool Opus request returned HTTP 400 with `parameters`, then HTTP 200, exact `SCHEMA_OK`, and `STOP` with `parametersJsonSchema` on the same working account. Initial comparisons also encountered capacity and stale-token errors, which are not counted as schema-fix successes.

See `investigation.md` for sanitized evidence and the upstream field documentation.

A bounded two-turn check after the schema fix returned one tool call, then failed on continuation with HTTP 400 `messages.1.content.1.tool_use.id: Field required`. The shared converter used by Antigravity omits both function-call and function-response identifiers; the separate converter used by the Google provider is not the same implementation. Antigravity stream parsing also replaces native IDs and reports ordinary STOP even after emitting a tool call.

## Goals / Non-Goals

**Goals:** Preserve the actual schema accepted by Claude Code tools, stop duplicate non-streaming replays of provider-declared HTTP client errors, and prove native Claude Code multi-turn execution for both advertised Claude models.

**Non-Goals:** Changing model aliases, OAuth state, upstream endpoint selection, Gemini schema semantics, general retry/backoff policies, or the user's MCP/plugin setup. No tool schemas or constraints are removed to make a request pass.

## Decisions

1. Select the schema field by the remapped upstream model. For `claude-` models, send the canonical non-nil schema as `parametersJsonSchema`, without Gemini sanitization. For other models, preserve the current `parameters` sanitizer path. This avoids applying an OpenAPI-only transform to a full JSON Schema and preserves constants and composition. The schema map is only marshalled, never modified. Keep existing omission for a nil schema.
2. Expose a narrow optional `NonStreamingFallbackAllowed() bool` method on the Antigravity upstream HTTP error. Return false for HTTP 4xx and preserve existing fallback eligibility otherwise. Shared route classification checks this interface through `errors.As` after its existing authentication guard. This avoids changing public status/error mapping or forcing unrelated providers to adopt a new error type.
3. Keep the existing provider-internal one-time 401 credential renewal. The new route restriction applies only after the adapter returns an error, preventing a second execution outside that bounded recovery.
4. Add a minimal, synthetic nested-union fixture based on the observed schema structure. Do not commit the full captured tool catalog, credentials, conversation content, or account-identifying diagnostic files. Cover references, constants, boolean schemas, required fields, nil schemas, immutability, model selection, streaming/collected execution, and the three ingestion dialects.
5. Verify real native tools, not only plain responses. Use a fresh loopback gateway with a new temporary home/database, seed only an explicitly approved working Antigravity credential snapshot, disable the exact-response cache, and run bounded isolated client configurations. Claude Code must perform five ordered read-only native tool calls with observed results and terminal success on both Opus and Sonnet, with SendMessage still present in its offered tool catalog. Run the corresponding available Codex, Droid, and GitHub Copilot CLI client checks or document concrete unavailable prerequisites; deterministic coverage for all four remains mandatory. Repeat Claude Code checks against the installed service after deployment.
6. Preserve `functionCall.id` and `functionResponse.id` in the shared converter actually used by Antigravity. Build a call-ID-to-name lookup while converting history and use it only when a result omits its name. Preserve native output `functionCall.id`; retain generated unique fallback IDs for missing values. Track emitted tool calls in the Antigravity SSE parser and prefer tool-use stop semantics over generic STOP or clean EOF. Keep stream errors visible. This repairs the actual call/result relationship rather than inventing unrelated IDs or dropping the previous turn.
7. Isolated Claude checks use the user's existing third-party-provider compatibility setting (`CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS=1`) and explicitly disable deferred tool search so the SendMessage schema is present on the wire. Retain request metadata to verify the intended model, output budget, and actual tool list. Verification harness failures, such as a missing PATH in a read-only Codex shell, are corrected in the harness and are not attributed to gateway code.

## Risks / Trade-offs

- Internal Cloud Code field support can differ from public documentation → controlled live comparison and actual native-client tool-loop verification gate deployment.
- Upstream capacity/authentication errors can obscure schema validation → record them separately, use bounded checks, and do not claim a successful model turn without visible terminal output.
- More permissive schema transport can expose previously stripped constraints → preserve canonical intent, exercise representative composition/reference schemas, and retain explicit upstream rejection for genuinely invalid schemas.
- Global fallback changes could affect other providers → use an opt-in error interface and retain existing helper tests and whole-repository race checks.
- Temporary verification credentials are sensitive → use private temporary directories, do not import the normal runtime database, never print credential values, and remove temporary credential state after testing.

## Migration Plan

After strict artifact validation and human approval, add failing regressions, implement the scoped changes, and run all required Bun/Go checks. Build the new binary for isolated verification, then back up the installed binary, install, restart `omnillm.service`, confirm health, and run the bounded native Claude checks. No schema migration or service-unit edit is needed. Rollback restores the backed-up binary and restarts the service. Record sanitized client call/result counts, terminal evidence, and any concrete skips before validated archival.
