## Why

OmniLLM currently writes truncated but otherwise unredacted tool arguments and tool results into structured tool-loop logs. Runtime evidence from 2026-08-02 through 2026-08-04 confirms credential-like content in these persistent fields, and errored Agent tool results can be emitted at warning level under the default log configuration, so debug-level gating and byte truncation do not provide an adequate privacy boundary.

## What Changes

- Make tool-loop diagnostics metadata-only by default across raw inbound, canonical inbound, non-streaming outbound, streaming outbound, and Agent tool-failure events.
- Prohibit structured logs from containing tool argument or tool result content, including truncated previews, while retaining operational metadata such as request correlation, tool identity, indexes, stream state, error state, and payload size.
- Remove parsing or accumulation performed solely to emit payload previews where it is no longer required for routing behavior.
- Add regression coverage with synthetic secret sentinels for nested arguments, plain and structured results, raw and canonical forms, streamed deltas, and warning-level Agent failures.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `security`: Define the logging privacy boundary that prevents tool payload content and credential material from entering structured logs at any enabled log level.
- `gateway-api`: Replace tool-loop payload previews with metadata-only diagnostics while preserving correlation and troubleshooting fields.

## Impact

- Affected code: `internal/routes/tool_loop_logging.go`, Anthropic Messages route call sites, and associated route logging tests.
- Affected outputs: structured stderr logs, the authenticated live log stream, and rotating JSON log files.
- Public request and response APIs remain unchanged; only diagnostic log fields change.
- No new dependencies or persistence migrations are required.
