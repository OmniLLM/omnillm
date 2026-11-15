## Context

The current provider commands call existing admin activation endpoints. Provider references, completion, and JSON output are shared with other operations. See `proposal.md` for motivation.

## Goals / Non-Goals

**Goals:** Make enable and disable the only CLI names while retaining existing active-set behavior.

**Non-Goals:** Rename server APIs, change provider state semantics, modify persistence, or deploy automatically.

## Decisions

- Rename the Cobra command uses to `enable` and `disable`, and remove the `activate` and `deactivate` aliases. The user has accepted the breaking change; retaining aliases was considered but rejected.
- Keep the existing endpoints and reference resolution. Update visible messages, diagnostics, and current documentation to use the canonical names. Separating CLI wording from API naming avoids server changes.
- Cover command help, canonical invocations, rejection of legacy names, requested paths, and JSON behavior with deterministic tests backed by a test server rather than changing real provider state.

## Risks / Trade-offs

- [Enable might be confused with model enabling] → Help text explicitly identifies the provider active set.
- [Scripts using the old names will fail] → Document migration to `enable` and `disable` and verify the old names cannot change provider state.

## Migration Plan

Update any callers from `activate` to `enable` and from `deactivate` to `disable`. Build and install the new binary through the normal workflow. If rollback is needed, reinstall the prior binary, which continues to accept the old names.
