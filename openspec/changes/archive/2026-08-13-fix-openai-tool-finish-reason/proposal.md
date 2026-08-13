## Why

The live four-model tool-call matrix showed that Copilot Claude Haiku 4.5 produces a canonical tool-use response, but the OpenAI-compatible response reports `finish_reason: stop`. Clients therefore treat a valid tool call as terminal text and cannot continue the tool-result turn.

## What Changes

- Serialize canonical tool-use responses as OpenAI `finish_reason: tool_calls` whenever tool calls are present.
- Make the real-server tool-loop compatibility script require the named tool when the scenario asserts a tool call, avoiding model-dependent automatic tool-choice behavior.
- Add regression coverage for mismatched upstream stop metadata and successful multi-turn tool replay.
- Rerun the live tool-call matrix and report provider/model outcomes.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-api`: OpenAI-compatible tool-call responses expose the finish reason required by OpenAI tool-loop clients.
- `cif-translation`: Canonical tool-use semantics take precedence over inconsistent upstream terminal metadata during OpenAI serialization.

## Impact

- Affected code: OpenAI response serialization and compatibility tests.
- Observable impact: tool-use responses previously labeled `stop` become `tool_calls`; ordinary terminal responses remain unchanged.
- No schema, dependency, provider configuration, or request-format changes.
