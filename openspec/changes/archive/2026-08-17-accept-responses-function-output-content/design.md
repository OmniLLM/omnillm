## Context

See `proposal.md` for motivation. OpenAI's Responses contract permits ordinary `function_call_output.output` to be either a string or an ordered content list containing text, image, and file items. OmniLLM's Responses ingestion currently accepts only a string, even though the adjacent custom-tool path already validates content lists, retains their original value in CIF, and computes compact JSON fallback text.

CIF tool-result `Content` is intentionally textual because Chat Completions, Anthropic, Gemini, logging, token estimation, and several translated provider paths consume strings. Native Responses providers can preserve structured output if CIF retains the original value separately. The solution must not reinterpret images/files as text, relax validation to arbitrary JSON, or change existing string results.

## Goals / Non-Goals

**Goals:**

- Accept exactly the ordinary function-output shapes allowed by the maintained Responses contract.
- Preserve structured output losslessly for native Responses replay.
- Keep a deterministic textual fallback for provider paths whose tool-result contract is string-only.
- Share validation and normalization logic with custom-tool output handling where semantics match.
- Prove behavior at ingestion, CIF marshaling, provider payload, route, and coding-client boundaries.

**Non-Goals:**

- Add new image/file support to providers that currently accept only textual tool results.
- Accept arbitrary object, scalar, or untyped array outputs beyond the Responses content-list contract.
- Change custom-tool wire semantics or ordinary string-result behavior.
- Add a new dependency or persistence migration.

## Decisions

### Preserve original output in CIF and retain compact JSON fallback

Add optional original-output metadata to canonical tool results for ordinary function results, parallel to existing custom output preservation. For string values, leave metadata unset and keep `Content` unchanged. For content lists, store the decoded ordered list and set `Content` to deterministic compact JSON.

This is preferred over changing `Content` from `string` to `interface{}` because the latter would ripple through every provider, serializer, logger, cache key, and token estimator and weaken the canonical textual fallback contract. It is preferred over stringifying without metadata because native Responses replay would become lossy and could no longer distinguish an original array from a caller-supplied JSON string.

### Use one typed content-list validator with context-specific errors

Extract or generalize the existing custom-output list validation so ordinary and custom results accept the same supported item vocabulary while returning errors that identify the relevant wire item. Validate every member before marshaling or mutating canonical messages.

Supported members are:

- `input_text` with its required text value;
- `input_image` with optional nullable image reference and detail fields, validated when present;
- `input_file` with optional nullable file reference, data, filename, and detail fields, validated when present.

Unknown types, non-object entries, absent discriminators, invalid supplied field types or enum values, and values exceeding maintained limits fail the whole request. The maintained schemas require only each member's `type`, so type-only image/file members and explicit nullable optional fields remain valid. Explicit empty strings and empty lists remain valid and distinct from an absent top-level `output` field.

This is preferred over accepting any JSON array because it would forward unverified content to providers and contradict fail-fast ingestion behavior.

### Replay structured values only on native Responses paths

Copilot, OpenAI-compatible Responses, and Azure Responses payload builders shall use the preserved original ordinary function output when present; otherwise they use `Content`. Custom results continue using their custom metadata and discriminator. Non-Responses provider builders use `Content`, preserving their existing string-only contracts.

This is preferred over emitting arrays into Chat Completions or Anthropic paths because those protocols have different content vocabularies and provider support varies.

### Cover both standards compliance and coding-agent continuation

Deterministic tests shall cover string, empty, text-only list, mixed text/image/file list, malformed entries, CIF marshal/unmarshal, native Responses payload preservation, translated text fallback, and route acceptance. Existing five-exchange fixtures remain the sustained client-loop baseline, with an additional structured function-result exchange in Responses-compatible fixtures so the reported failure cannot regress unnoticed.

Live client smokes remain isolated and bounded. Claude Code, Codex CLI, Droid, and GitHub Copilot CLI custom-provider mode run five ordered native calls when available. A separate direct live Responses probe proves structured-output continuation if installed clients cannot be made to emit a content array deterministically; that limitation is recorded rather than misreported as client evidence.

## Risks / Trade-offs

- **[Structured metadata increases CIF surface]** → Keep it optional, omit it for legacy/string results, and add marshal/unmarshal compatibility tests.
- **[Provider-specific array support may differ]** → Replay arrays only for adapters that explicitly use the Responses API; preserve deterministic textual fallback everywhere else.
- **[Image/file validation can drift from upstream schema]** → Reuse the maintained input-content validation vocabulary and add exact accepted/rejected fixtures rather than duplicating permissive checks.
- **[A real client may not emit arrays during a bounded smoke]** → Require deterministic route coverage and a direct credentialed Responses continuation probe; record native-client structured-output skips precisely.
- **[Existing running gateway can hide stale binary behavior]** → Build and launch verification on a fresh loopback port with temporary state and report the actual executable/version and port.

## Migration Plan

1. Land the backward-compatible CIF field and ingestion normalization together so no intermediate build loses structured output.
2. Update all native Responses provider builders in the same change.
3. Run deterministic and full verification, then launch a fresh isolated gateway for direct and native-client live smokes.
4. Rollback is a normal code revert; there is no schema, dependency, or persisted-data migration.
