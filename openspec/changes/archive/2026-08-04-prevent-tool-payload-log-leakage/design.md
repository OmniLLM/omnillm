## Context

See `proposal.md` for motivation. Anthropic Messages handling currently extracts raw and canonical tool results and accumulates outbound tool arguments for structured diagnostics. The same enabled event is fanned out to stderr, the live SSE log stream, and a rotating JSON file, so payload privacy must be enforced before constructing the log event rather than independently at each destination.

The current 400-byte, UTF-8-safe truncation helper is an integrity and size control, not a confidentiality control. Tool payloads are arbitrary strings or JSON and cannot be made reliably safe by redacting selected key names.

## Goals / Non-Goals

**Goals:**

- Establish one privacy invariant for every tool-loop diagnostic path and log level.
- Preserve metadata that identifies the request, tool call, location in the turn, model/provider, stream state, and error state.
- Preserve payload size as a useful diagnostic without retaining payload content.
- Cover raw, canonical, non-streaming, streaming, and Agent failure paths with sentinel-based regression tests.

**Non-Goals:**

- Introduce a payload-logging opt-in or a new log destination.
- Redact arbitrary request, response, or provider logs outside tool-loop diagnostics.
- Change tool execution, CIF translation, provider dispatch, or public API behavior.
- Delete or rewrite log files that already exist on an operator's machine.

## Decisions

### Tool-loop events are metadata-only at the source

Remove payload-valued fields from event construction before zerolog sends records to any writer. Retain existing non-payload fields and add byte-length fields where the content is already available.

This is preferred over destination filters because every destination receives the same event and a future destination could otherwise omit the filter. It is preferred over best-effort redaction because tool results may be opaque command output, file content, or prose with no recognizable sensitive key.

### Length replaces content previews

Use explicit argument/result byte counts as the only payload-derived diagnostic values. Byte counts help distinguish empty, small, and truncated/large interactions without creating a reversible representation.

Digests are excluded: although non-reversible in the normal case, they provide little operational value here, may permit guessing low-entropy payloads, and complicate the privacy contract.

### Raw and canonical event structure remains intact

Keep both inbound event types for now because they diagnose different ingestion stages. Remove content from both so duplicate events no longer duplicate sensitive data. Eliminating one event is a separate observability-volume decision.

### Streaming accumulation is minimized, not repurposed

Preserve the streaming tool-call tracker because it supplies tool identity, block indexes, and Agent guardrail state. Remove only raw-argument or partial-JSON accumulation that exists solely to create a log preview; retain any accumulation required to produce the actual response or canonical tool call. Compute length incrementally from streamed bytes where possible, and compute canonical result length before discarding its content from the diagnostic representation.

### Privacy tests assert record-wide absence

Capture zerolog output and search the complete serialized record set for unique synthetic sentinels, rather than checking only that named fields are absent. This catches accidental leakage through renamed fields, messages, nested structures, raw/canonical duplicates, and warning-level events. Tests also assert that expected metadata remains present.

## Risks / Trade-offs

- [Operators lose direct payload previews used for debugging malformed tool calls] → Preserve correlation, indexes, error state, and byte lengths; payload reproduction must occur outside persistent application logs with deliberately controlled test inputs.
- [A payload could be copied into an error string by a different logging path] → Use record-wide sentinel assertions across representative route flows and expand coverage if another emitter is discovered.
- [Removing preview-only parsing changes control flow accidentally] → Separate extraction cleanup from event-field removal and retain route-level behavior tests for raw/canonical and streamed tool calls.
- [Existing logs continue to contain old payload previews] → Document that deployment prevents new records only; operators control retention and deletion of historical local logs.

## Migration Plan

1. Deploy the metadata-only event changes with regression tests.
2. No configuration or database migration is required; existing log consumers must tolerate removal of `tool_arguments`, `tool_result`, and `raw_inbound_payload` payload fields and may consume new byte-length fields.
3. Rollback is a normal binary rollback, but it reintroduces payload logging and is not recommended except for controlled diagnosis with non-sensitive data.
4. Historical rotating log files are not modified automatically.
