## MODIFIED Requirements

### Requirement: Provider lifecycle management

The system SHALL expose authenticated operations to list, log in or
re-authenticate, configure, prioritize, activate, deactivate, rename, and delete
provider instances and to manage their model catalogs. Provider-scoped
operations MUST accept a provider reference resolved from instance ID, alias, or
display name, and the normalized login operation MUST support both new-instance
authentication by provider type and existing-instance re-authentication.

#### Scenario: Normalized immediate login

- **WHEN** an authenticated operator submits credentials that can be validated
  synchronously for a supported new provider type or existing provider reference
- **THEN** the login operation returns a common completed result containing the
  canonical provider instance ID and whether the instance was newly created

#### Scenario: Normalized interactive login

- **WHEN** authentication requires browser or device authorization
- **THEN** the login operation returns a common pending result containing the
  canonical provider instance ID, authorization instructions, and a flow token
  usable with one status operation until completion, failure, cancellation, or
  expiry

#### Scenario: Existing-provider re-authentication

- **WHEN** a login target resolves to an existing provider by ID, alias, or name
- **THEN** successful authentication updates that instance's credentials and
  preserves its canonical ID, alias, display name, activation, priority, and
  model configuration

#### Scenario: Failed new-provider login

- **WHEN** authentication or persistence fails after a new provider instance is
  reserved
- **THEN** the reserved parent and its owned partial records are removed

#### Scenario: Failed existing-provider login

- **WHEN** re-authentication of an existing provider fails
- **THEN** the existing provider instance and its prior durable metadata are not
  deleted

#### Scenario: Provider reference in lifecycle operation

- **WHEN** an authenticated operator supplies a unique provider alias or display
  name to a provider-scoped lifecycle or model-catalog operation
- **THEN** the operation applies to the resolved canonical instance ID and the
  response identifies that ID

#### Scenario: Legacy authentication endpoint

- **WHEN** an existing client uses a previous create, re-authentication, OAuth
  start, callback, or status endpoint
- **THEN** the existing request and response contract remains available

#### Scenario: Provider list identity fields

- **WHEN** an authenticated operator lists provider instances
- **THEN** every item contains its canonical `id`, display `name`, canonical
  `alias`, and the compatibility `subtitle` value equal to `alias`

#### Scenario: Provider metadata update

- **WHEN** an authenticated operator updates `alias` or the compatibility field
  `subtitle`
- **THEN** the same persisted alias is updated and returned as both fields

#### Scenario: Provider deactivation

- **WHEN** an authenticated operator deactivates an existing provider
- **THEN** the provider is excluded from the active provider set

#### Scenario: Model catalog refresh

- **WHEN** an authenticated operator requests a provider model refresh
- **THEN** the provider cache is bypassed and the persisted model view is updated
