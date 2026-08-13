## Context

A live Copilot Claude response contained canonical tool-call content and was logged with `stop_reason=tool_use`, yet OpenAI serialization emitted `finish_reason=stop`. The response serializer currently trusts stop metadata independently from tool-call content, allowing an inconsistent envelope.

## Goals / Non-Goals

**Goals:**

- Make OpenAI-compatible tool responses self-consistent.
- Preserve valid tool calls and enable the existing replay request format.
- Cover the exact live failure with a deterministic regression test.

**Non-Goals:**

- Change provider prompting or force models to call tools.
- Change Anthropic or Responses API finish semantics.
- Treat malformed or empty tool-call structures as successful tool use.

## Decisions

### Normalize at the OpenAI serialization boundary

When a canonical response contains valid tool calls, OpenAI serialization will emit `tool_calls` regardless of a generic stop value. This boundary owns the OpenAI vocabulary and can enforce envelope consistency without changing provider-specific parsing.

The alternative—patching only Copilot Claude parsing—would leave the same inconsistency possible for another provider and would duplicate a format-level invariant in provider code.

### Preserve ordinary stop mapping

Responses without tool calls retain existing stop-reason mapping. The correction is narrow and does not infer tool use from text or incomplete metadata.

## Risks / Trade-offs

- **[A provider includes tool calls it intended as non-actionable metadata]** → Only canonical tool-call content already exposed to clients triggers normalization; such content is actionable under the existing CIF contract.
- **[Streaming has a separate finish path]** → Existing streaming tests remain in the full suite; this change targets the observed non-streaming live defect.
