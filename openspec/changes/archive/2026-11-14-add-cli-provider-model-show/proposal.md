## Why

Operators can list providers and models, but cannot inspect one provider or one
model without manually filtering JSON. Detail commands make existing catalog
information discoverable and usable by both people and scripts.

## What Changes

- Add `omnillm provider show <reference>` and `omnillm provider model show
  <reference> <model-id>` with table and JSON output.
- Resolve provider references using the established ID, alias, and name
  precedence; select model IDs exactly, including IDs containing slashes.
- Display available provider status and model catalog details without printing
  credential-bearing configuration.
- Document the commands and add deterministic CLI regression tests.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `cli-ops-config`: add targeted provider and model inspection commands with
  safe detail output, reference handling, and actionable errors.

## Impact

This changes the public CLI behavior and help. It reuses existing authenticated
provider and model list APIs; no API, persistence, routing, provider discovery,
or dependency contract changes are intended. Existing commands remain intact.
