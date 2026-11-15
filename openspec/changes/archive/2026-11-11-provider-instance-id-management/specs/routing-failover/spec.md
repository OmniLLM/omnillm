## ADDED Requirements

### Requirement: Routing after provider identity change
After a successful provider ID change, routing SHALL use the new ID for active and primary provider selection and for virtual-model upstreams that explicitly referenced the old canonical ID. It SHALL invalidate obsolete provider-owned routing caches and SHALL NOT change native model identifiers merely because they begin with the old ID. The old ID SHALL NOT be retained as an automatic redirect; existing alias and display-name resolution remains unchanged.

#### Scenario: Active virtual-model upstream
- **WHEN** an active provider referenced by canonical ID in a virtual-model upstream is renamed
- **THEN** the upstream continues to route to the same provider under its new ID without a restart

#### Scenario: Native model identifier resembles an old prefix
- **WHEN** a stored or requested native model ID starts with the old provider ID followed by a slash
- **THEN** the rename does not blindly rewrite the native model ID

#### Scenario: Old external explicit reference
- **WHEN** an external client uses the old ID as a provider reference after rename
- **THEN** it is not automatically redirected to the renamed provider, although existing alias or display-name resolution still applies
