# Design: Interactive Provider Rename

## Context

Provider reference selection is interactive when the positional reference is
omitted, but rename metadata is currently accepted only through flags. The CLI
therefore selects a provider successfully and immediately returns `at least one
of --name or --alias is required`.

## Decision

Keep flags authoritative. After provider resolution and alias/subtitle conflict
validation, prompt for both `New display name` and `New provider alias` only
when all rename metadata flags are empty and command output is attached to a
terminal. Trim prompted input and include only non-empty fields in the existing
PATCH request.

Use command-scoped input/output streams for the prompt path so deterministic
tests and embedding callers can control I/O. If both prompted values are empty,
return the existing validation error and do not contact the rename endpoint.

## Compatibility

Explicit flags never cause a prompt. Non-interactive use without metadata keeps
failing immediately, preventing pipelines and automation from hanging. The
hidden `--subtitle` alias and its conflict rule remain unchanged. No server API
or stored data changes are required.

## Failure Handling

Prompt cancellation or input failure is returned as a command error. Empty
prompt results are rejected locally. HTTP and server errors retain their current
behavior.
