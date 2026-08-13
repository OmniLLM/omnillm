## Why

Droid/factory-cli 0.195.0 replays freeform tool history through the Responses API as `custom_tool_call` and `custom_tool_call_output` items. OmniLLM currently rejects the first custom call during inbound translation, returning HTTP 400 before provider selection and breaking otherwise successful Droid tool loops against `jzhu/gpt-5.6-sol`.

## What Changes

- Accept explicitly typed Responses custom tool calls and outputs during `/v1/responses` ingestion.
- Normalize each custom call into the existing CIF tool-call vocabulary while preserving its call ID, name, ordering, and raw input as one required string argument named `input`.
- Normalize string outputs unchanged and ordered supported content-list outputs as compact JSON text while preserving call/result association.
- Normalize custom tool definitions into existing CIF function-tool definitions with a required string `input` parameter.
- Add deterministic Droid-style ingestion and route compatibility coverage based on a sanitized `ApplyPatch` history.
- Establish repository guidance requiring affected tool-call changes to verify Claude Code, Codex CLI, and Droid multi-turn tool use, including bounded live smoke tests when local client/model configurations are available.
- Reject malformed custom items with structured invalid-request errors rather than silently dropping data.
- Keep native/lossless custom-tool identity, custom formats, provider emission, and custom streaming events out of scope.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `cif-translation`: Define focused normalization of Responses custom-tool history into existing canonical tool calls and results.
- `gateway-api`: Require `/v1/responses` to accept valid custom-tool history and reject malformed custom items predictably.
- `model-compatibility-testing`: Add deterministic Droid-style custom-tool history coverage through Responses ingestion and routing.

## Impact

- Inbound Responses parsing and normalization in `internal/ingestion/from_responses.go`.
- Focused parser tests in `internal/ingestion/responses_test.go`.
- Shared compatibility fixtures and assertions under `internal/testcompat`, `internal/ingestion`, and `internal/server`.
- Project agent guidance in `CLAUDE.md` for Claude Code, Codex CLI, and Droid multi-turn tool-call verification.
- No dependency, database, configuration, CIF type, provider adapter, or outbound serialization changes.
