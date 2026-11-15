## ADDED Requirements

### Requirement: Atomic provider identity transfer
A successful provider ID change SHALL atomically transfer the provider instance, credentials, provider configuration, model states, model configurations, model-list cache, and exact canonical-ID references in virtual-model upstream provider fields to the new ID. It SHALL preserve name, alias, priority, activation, timestamps other than update timestamps, and unrelated records. A failed transfer MUST leave the prior durable state intact. Historical request-log attribution SHALL remain unchanged.

#### Scenario: Provider with dependent records
- **WHEN** an instance with credentials, model settings, and a virtual-model upstream is renamed
- **THEN** those records belong to or refer to the new ID and remain usable after restart

#### Scenario: Transaction fails
- **WHEN** a dependent record cannot be transferred
- **THEN** the transaction rolls back and the original provider and all of its records remain available under the old ID

#### Scenario: Historical metering
- **WHEN** a provider is renamed after serving requests
- **THEN** existing request-log rows retain the provider ID recorded at the time of those requests
