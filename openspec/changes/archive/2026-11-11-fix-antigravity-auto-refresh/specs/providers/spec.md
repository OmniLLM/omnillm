## ADDED Requirements
### Requirement: Antigravity automatic credential refresh
Antigravity SHALL refresh OAuth access tokens before generation when expiry is within 60 seconds, and SHALL recover legacy credentials with unknown expiry when refresh credentials are available. Concurrent callers MUST coordinate refresh and reuse renewed credentials. Explicit refresh SHALL use the same safe refresh mechanism.

#### Scenario: Expiring access token
- **WHEN** generation starts with an expiring access token and valid refresh credentials
- **THEN** the provider refreshes and persists credentials before issuing generation with the renewed token

#### Scenario: Legacy OAuth record
- **WHEN** a stored OAuth record has refresh credentials but no expiry
- **THEN** generation obtains renewed credentials without requiring an additional interactive login

#### Scenario: Concurrent generation
- **WHEN** multiple requests encounter the same expired credential snapshot
- **THEN** they coordinate renewal and reuse the resulting credentials instead of exchanging the same snapshot independently

#### Scenario: Refresh unavailable or rejected
- **WHEN** authentication recovery requires missing refresh credentials or the refresh grant is rejected
- **THEN** the request returns a sanitized re-authentication error without exposing tokens or overwriting valid newer credentials

#### Scenario: Cancellation or storage failure
- **WHEN** refresh is cancelled or reading or persisting credentials fails
- **THEN** execution returns an error without issuing generation using stale or unpersisted refreshed credentials

### Requirement: Antigravity bounded authentication retry
Antigravity SHALL retry generation at most once after an upstream HTTP 401 received before stream delivery, using refreshed or concurrently renewed credentials. It MUST preserve the original tools and history and MUST NOT retry after output delivery, on cancellation, on other HTTP failures, or after a second 401.

#### Scenario: Rejected access token
- **WHEN** upstream rejects a generation request with HTTP 401 before streaming begins
- **THEN** the provider renews credentials or reuses a newer token and retries the same request once

#### Scenario: Retry rejected
- **WHEN** the retried request also returns HTTP 401
- **THEN** that failure is returned without another refresh-and-retry cycle

#### Scenario: Stream already started
- **WHEN** a failure occurs after stream delivery starts
- **THEN** the error is surfaced without replaying generation or duplicating events

#### Scenario: Non-authentication failure
- **WHEN** upstream returns HTTP 403 or a non-authentication error
- **THEN** no authentication retry occurs
