## Why

Two-call smoke tests proved basic continuation but were too weak to establish sustained agent-loop compatibility. Tool-call changes must withstand at least five sequential call/result cycles across every supported coding-agent client, including GitHub Copilot CLI's custom-provider mode.

## What Changes

- Raise the deterministic sequential tool-loop minimum from three calls to five calls plus terminal continuation.
- Require five-call deterministic and bounded live validation for Claude Code, Codex CLI, Droid, and GitHub Copilot CLI custom provider.
- Require distinct call IDs and markers, ordered call/result association, no parallel batching, terminal assistant output, successful client exit, and no sixth call.
- Expand the shared compatibility fixture and live repeated-tool runner from three to five cycles.
- Update project guidance and testing documentation with the permanent four-client validation rule.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `model-compatibility-testing`: Raise sequential and coding-client tool-loop acceptance to five calls and include GitHub Copilot CLI custom-provider coverage.

## Impact

- Shared deterministic compatibility fixtures and manifest assertions.
- Live model-matrix repeated-tool logic and tests.
- Project `CLAUDE.md` and compatibility-testing documentation.
- Local opt-in smoke execution for four installed coding-agent CLIs; no runtime product behavior or dependency changes.
