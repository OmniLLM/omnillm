## Why

Interactive provider and virtual-model selection menus currently display only
identifiers, which makes UUID-based provider instances difficult to distinguish.
Operators need recognizable names and aliases to choose the intended item safely.

## What Changes

- Show available provider display names and aliases alongside their full IDs in
  every interactive provider-instance picker, including `provider show`,
  `provider enable`, and provider-model operations.
- Show available virtual-model display names alongside their full IDs in every
  interactive virtual-model picker.
- Allow searching by displayed identity fields while keeping the selected
  canonical ID separate from its label. Retain safe ID-only fallback when
  descriptive metadata is absent.
- Audit other interactive selections for identifier-only choices and preserve
  existing explicit-argument, non-interactive, empty-list, single-item, and
  cancellation behavior.
- Add deterministic regression coverage for display, search, duplicate names,
  missing metadata, and the IDs used by selected operations.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `cli-ops-config`: interactive entity selectors provide readable identity
  metadata without changing the selected entity or existing automation behavior.

## Impact

This changes interactive CLI presentation and search only. The existing admin
API responses provide the necessary metadata; no server API, persistence,
provider identity, explicit command argument, or JSON output contract changes.
