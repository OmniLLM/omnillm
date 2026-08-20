## ADDED Requirements

### Requirement: Rejected provider refresh-token retirement

Provider-owned credential persistence SHALL durably remove a refresh token
that its authoritative OAuth endpoint identifies as already consumed when no
different newer token is available, while preserving the provider's remaining
credential and identity fields.

#### Scenario: Consumed token remains current

- **WHEN** an OAuth refresh token is rejected as already consumed and the same
  token remains in the provider's durable credential record
- **THEN** persistence removes that refresh token without deleting the access
  token, account identity, or provider instance

#### Scenario: Newer token won the race

- **WHEN** an OAuth refresh token is rejected as already consumed and a
  different refresh token is already durable
- **THEN** the newer token is preserved and is not overwritten by retirement
  of the rejected token
