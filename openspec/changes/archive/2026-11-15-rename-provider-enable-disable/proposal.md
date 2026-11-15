## Why

Provider activation commands use terminology inconsistent with the requested enable/disable interface. Use one canonical pair of command names without maintaining legacy aliases.

## What Changes

- Advertise `omnillm provider enable` and `omnillm provider disable` for adding and removing providers from the active set.
- **BREAKING:** Remove the `activate` and `deactivate` CLI aliases; callers must use `enable` and `disable` respectively.
- Update CLI messages, documentation, diagnostics, and tests to use the canonical names.
- Preserve existing administrative API endpoints and provider activation semantics.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `cli-ops-config`: Canonical provider active-set commands and rejection of legacy command names.

## Impact

The Go CLI command surface, CLI help and diagnostics, documentation, and tests change. Scripts using the old CLI names must migrate. Existing API endpoints, provider state semantics, and storage remain unchanged.
