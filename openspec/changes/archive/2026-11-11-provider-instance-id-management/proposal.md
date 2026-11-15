## Why

Provider instance IDs currently vary in length and may be derived from provider types, accounts, endpoints, or credential fragments. Operators cannot edit the canonical ID, making provider lists inconsistent and preventing them from choosing a more convenient identifier.

## What Changes

- Generate a fixed-length random UUID for each newly created provider instance, across immediate and interactive authentication paths, without deriving IDs from account or credential data.
- Allow operators to change an existing instance ID from the Providers workspace through the authenticated administration API. Validate the new ID, reject collisions, and preserve the instance's owned state and internal routing references.
- Leave existing IDs unchanged on upgrade and re-authentication. Warn that external clients and configuration that explicitly use an old ID must be updated after a rename.
- Preserve existing display-name and alias editing and provider-reference behavior. Historical request logs retain their original attribution.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `providers`: Fixed-length generated instance IDs and safely changeable canonical identities.
- `admin-api`: Authenticated instance-ID updates and compatible provider login behavior.
- `admin-ui`: Provider instance-ID editing and warnings in the Providers workspace.
- `persistence`: Atomic transfer of provider-owned state and internal references when an instance ID changes.
- `routing-failover`: Preserve internal provider routing after an instance ID change and invalidate obsolete cache ownership.

## Impact

Provider creation and OAuth flows, registry lifecycle and synchronization, SQLite provider-owned records and virtual-model references, provider reference resolution, and the administration API and console are affected. Existing databases and existing provider IDs remain valid. Renaming an ID is an explicit breaking change for external references to that ID; they are outside OmniLLM's control and must be updated by the operator. This change alters runtime behavior and public administration API behavior.
