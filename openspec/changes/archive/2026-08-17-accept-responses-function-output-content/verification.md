# Verification Evidence

## Approved specification

- Change: `accept-responses-function-output-content`
- Strict validation: `bun run spec:validate`
- Result: 15 passed, 0 failed
- Human approval: initial artifacts approved, then standards-alignment revision approved after review against the current official OpenAI OpenAPI schema.

## Deterministic compatibility

- Focused command: `go test ./internal/cif ./internal/ingestion ./internal/testcompat ./internal/server ./internal/providers/copilot ./internal/providers/openaicompat ./internal/providers/openaicompatprovider ./internal/providers/azure ./internal/providers/shared -count=1`
- Result: pass
- Covered output forms: string, empty string, empty list, ordered text list, mixed text/image/file list, type-only image/file members, explicitly nullable optional fields, missing output, missing call identifier, invalid scalar/object/member/type/field/detail values, schema length limits counted as Unicode code points, and unchanged legacy custom-output semantics.
- Covered boundaries: Responses ingestion, CIF metadata and fallback text, five ordered structured result cycles, public 400 envelope, native Copilot/OpenAI-compatible/Azure Responses replay, Chat Completions fallback text, Gemini fallback text, and fake-upstream terminal continuation.

## Fresh-port credentialed structured continuation

- Patched binary: built from this working tree.
- Fresh gateway: temporary HOME/config/database on `127.0.0.1:39134`.
- Model route: authenticated `jzhu/gpt-5.6-sol` via a temporary loopback compatibility bridge to the existing live route.
- Bridge purpose: record that the patched gateway emitted a structured output array, then stringify only for the older upstream OmniLLM binary that still rejects arrays.
- Evidence: HTTP 200; response status `completed`; structured array observed upstream; terminal text `STRUCTURED_LIVE_DONE`; unexpected follow-up calls 0.
- Cleanup: temporary gateway, bridge, settings, token files, binary, and state directory removed.

## Native coding clients on the fresh patched gateway

| Client | Version | Shape / native tool | Calls | Results | Terminal | Exit | Outcome |
|---|---:|---|---:|---:|---|---:|---|
| Claude Code | 2.1.233 | Anthropic Messages / `Bash` | 5 | 5 | `CLAUDE_PATCHED_DONE` | 0 | pass |
| Codex CLI | 0.147.0 | Responses custom / `exec` | 5 | 5 | `CODEX_DIRECT_DONE` | 0 | pass |
| Droid | 0.197.0 | Responses / `Execute` | 5 | 5 | `DROID_PATCHED_DONE` | 0 | pass |
| GitHub Copilot CLI | 1.0.81-0 | Responses custom provider / `shell` | 5 | 5 observed in order by CLI display | `COPILOT_PATCHED_DONE` | 0 | pass |

Each pass used markers 1 through 5 in distinct sequential calls, observed each result before the next call, made no sixth call, and reached a terminal response. Codex was rerun through a directly provisioned temporary GitHub Copilot provider because the generic compatibility bridge cannot preserve native custom-tool payload kind; the direct run passed.

## Complete verification

- `gofmt`: pass
- `go vet ./...`: pass
- `go build ./...`: pass
- `go test -race ./...`: pass
- `bun run lint:all`: pass; dependency freshness warning only
- `bun run typecheck`: pass
- `bun test`: 358 passed, 22 skipped, 0 failed
- `bun run build`: pass; existing chunk-size warning only
- `bun run spec:check`: pass; 15 strict items validated and mandatory spec gate passed

## Review

- Simplification review applied clearer numeric marker formatting and explicit empty-output table cases.
- Correctness review identified the schema-alignment issue for optional image/file fields; the artifacts and implementation were revised and reapproved.
- A later pass identified Unicode code-point length semantics and accidental custom-output tightening; both were fixed with targeted regressions.
- Final correctness review verified both fixes and found no remaining high-confidence actionable findings.
