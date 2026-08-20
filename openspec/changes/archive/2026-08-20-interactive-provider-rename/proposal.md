# Make Provider Rename Interactive

## Why

`omnillm provider rename` already lets an operator select a provider when the
provider reference is omitted, but it then fails unless `--name` or `--alias`
was supplied before the selection. This produces a dead end in an otherwise
interactive workflow.

## What Changes

- When `provider rename` runs in an interactive terminal without `--name` or
  `--alias`, prompt for the new display name and alias after resolving the
  provider.
- Require at least one non-empty prompted value before sending the update.
- Preserve explicit `--name` and `--alias` behavior, including the hidden
  `--subtitle` compatibility spelling.
- Preserve validation-only failure when neither rename field is supplied in a
  non-interactive invocation so automation never blocks on a prompt.
- Add focused CLI regression coverage for interactive input and non-interactive
  compatibility.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `cli-ops-config`: extend provider rename with an interactive metadata prompt
  while retaining script-safe flag behavior.

## Runtime Behavior Impact

This change affects only the interactive CLI flow for `provider rename` when no
rename flags are supplied. The administration API, persistence contract,
provider identity resolution, and flag-driven command behavior are unchanged.
