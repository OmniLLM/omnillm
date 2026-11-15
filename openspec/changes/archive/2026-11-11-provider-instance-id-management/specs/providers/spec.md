## ADDED Requirements

### Requirement: Fixed-length provider instance IDs
The system SHALL assign a cryptographically random UUID version 4 in canonical 36-character form to every newly created provider instance, independently of provider type, account, endpoint, and credentials. It SHALL reserve IDs uniquely without replacing existing instances and SHALL preserve the assigned ID throughout authentication, re-authentication, refresh, and restart.

#### Scenario: Immediate and interactive provider creation
- **WHEN** an operator successfully creates any supported provider using immediate or interactive authentication
- **THEN** its canonical ID is a unique 36-character UUID and all successful authentication results identify the same instance

#### Scenario: Concurrent creation or collision
- **WHEN** simultaneous creations request an ID or a generated ID already exists
- **THEN** each successful creation has a distinct ID and no existing instance is overwritten

#### Scenario: Existing instance re-authentication
- **WHEN** an existing provider is re-authenticated or refreshes credentials
- **THEN** its canonical ID remains unchanged regardless of whether it is a UUID or a legacy ID

#### Scenario: Existing database upgrade
- **WHEN** a database with legacy provider IDs is opened after upgrade
- **THEN** those IDs remain unchanged and continue to resolve

### Requirement: Provider identity change
The system SHALL allow an operator to change an existing canonical instance ID without changing its provider type, name, alias, priority, activation, credentials, or model configuration. It SHALL reject an invalid or conflicting ID and SHALL NOT leave a partially changed provider on failure.

#### Scenario: Successful identity change
- **WHEN** a valid unused ID is assigned to an existing provider
- **THEN** the same provider is available under the new ID with its existing state and remains usable after restart

#### Scenario: Rename during conflicting work
- **WHEN** an instance cannot be safely renamed because authentication or other provider work is in progress
- **THEN** the system rejects the rename without modifying the instance and provides a retryable explanation
