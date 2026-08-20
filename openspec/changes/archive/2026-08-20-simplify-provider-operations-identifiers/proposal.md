# Simplify Provider Operations and Identifiers

## Why

Provider administration currently exposes authentication twice at the CLI root
(`auth` and `provider add`), separates provider models into another root command,
and uses different backend paths for create, re-authentication, and OAuth
providers. Operators must already know whether they are creating or refreshing
credentials before choosing a command or endpoint.

Provider references are also inconsistent. Gateway model routing accepts an
instance ID or the persisted `subtitle`, while administration commands and
endpoints generally require only the instance ID. The UI calls the same value a
subtitle, even though users treat it as a short alias. Display names cannot be
used for `provider/model` routing.

## What Changes

- Make `provider` the canonical CLI namespace:
  - `omnillm provider login [type-or-provider]` creates a provider when given a
    provider type and re-authenticates an existing provider when given its ID,
    name, or alias.
  - `omnillm provider model ...` contains provider model operations.
  - Existing root `auth`, root `model`, and `provider add` invocations remain
    accepted as deprecated, hidden compatibility shims.
- Add one normalized provider-login administration operation that selects
  create versus re-authentication from a provider type or provider reference
  and returns a consistent immediate-success or pending-authentication shape.
  Existing authentication endpoints remain available for compatibility.
- Define a shared provider-reference resolver for administration and routing.
  It resolves exact instance ID first, then a unique case-insensitive alias,
  then a unique case-insensitive display name. Ambiguous references fail and
  report the matching instance IDs.
- Promote `alias` as the public term for the existing persisted `subtitle`.
  Provider responses expose `alias`; request bodies accept `alias`; the legacy
  `subtitle` field remains accepted and emitted during the compatibility period.
- Allow gateway clients to select a provider with `ID/model-id`,
  `name/model-id`, or `alias/model-id`, preserving native slash-containing model
  identifiers and the existing native-model-first collision rule.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `cli-ops-config`: consolidate authentication and model management under the
  provider command while retaining hidden compatibility shims.
- `admin-api`: provide a normalized login operation and accept provider
  references across provider-scoped lifecycle and model operations.
- `providers`: define ID, display name, and alias identity metadata and
  deterministic reference resolution.
- `routing-failover`: extend provider-qualified model routing from IDs and
  subtitle aliases to IDs, aliases, and display names.

## Runtime Behavior Impact

This change adds public CLI, administration API, provider response, and gateway
model-routing behavior. Existing instance IDs, persisted subtitle values,
authentication endpoints, CLI commands, and provider-qualified model strings
remain compatible. No provider credentials or instance IDs are rewritten, and
no database migration is required.
