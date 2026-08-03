## ADDED Requirements

### Requirement: Copilot transient transport retry
GitHub Copilot chat-completions requests SHALL retry exactly once when the upstream attempt fails with a transient transport failure before any stream event has been emitted to the caller. A transient transport failure SHALL mean a connection-lost, connection-reset, or unexpected-EOF transport error, an HTTP/2 `INTERNAL_ERROR` stream reset, or an upstream response status of 502, 503, or 504. The retry SHALL be delayed by a short randomized interval. Timeout failures SHALL NOT be treated as transient transport failures, and no failure class SHALL be retried more than once.

#### Scenario: Connection lost before first event
- **WHEN** a Copilot streaming chat-completions request fails with a lost or reset connection before any stream event is emitted
- **THEN** the request is re-issued once after a randomized delay and the caller receives the successful retry result as a single uninterrupted stream

#### Scenario: Upstream service unavailable
- **WHEN** a Copilot chat-completions request receives upstream status 503 before any stream event is emitted
- **THEN** the request is re-issued once rather than failing the caller immediately

#### Scenario: Failure after first event
- **WHEN** a Copilot streaming request fails after at least one stream event has been emitted
- **THEN** no retry is attempted and the error surfaces to the caller, so already-delivered output is never duplicated

#### Scenario: Timeout is not retried
- **WHEN** a Copilot request fails by exceeding its configured response-header budget
- **THEN** the attempt fails once with no automatic duplicate request, preserving the existing timeout contract

#### Scenario: Retry also fails
- **WHEN** both the initial attempt and its single retry fail
- **THEN** the error from the final attempt is returned to provider dispatch, which may proceed to its next candidate

#### Scenario: Non-transient error
- **WHEN** a Copilot request fails with a non-transient error such as status 400 or an authentication failure
- **THEN** no transport retry is attempted and existing error handling applies unchanged

### Requirement: Copilot transport retry diagnostics
A Copilot transient transport retry SHALL emit one structured warning containing the provider instance, upstream endpoint, canonical model, attempt number, and the classification reason that triggered the retry, without including credentials or request content.

#### Scenario: Retry logged
- **WHEN** a Copilot request is retried after a transient transport failure
- **THEN** a single warning records the provider, endpoint, model, attempt number, and why the failure was classified as transient
