## ADDED Requirements

### Requirement: Native System One evaluation endpoint
The gateway SHALL expose `POST /v1/systemone` using the existing proxy
authentication and post-authentication concurrency controls. Requests SHALL contain
a non-empty model, a string/object/array state, and a non-empty map of Noul, Choice,
or Score questions following the TypeSafe HTTP contract. Successful responses SHALL
preserve the native model, answers, and usage JSON shape.

#### Scenario: Native evaluation
- **WHEN** an authenticated client posts a valid request for an active TypeSafe model
- **THEN** the gateway returns the native structured evaluation result as JSON

#### Scenario: Missing inbound credentials
- **WHEN** gateway authentication is enabled and an evaluation request lacks accepted credentials
- **THEN** the gateway returns HTTP 401 without contacting TypeSafe or occupying a concurrency slot

#### Scenario: Concurrency exhausted
- **WHEN** an authenticated evaluation arrives while the proxy concurrency limit is exhausted
- **THEN** the gateway returns HTTP 503 with `Retry-After: 1`

### Requirement: System One input and error handling
The System One endpoint SHALL reject malformed JSON, invalid required fields,
unsupported primitive shapes, unknown top-level fields, and generation-only fields
such as messages, tools, or stream with HTTP 400 before upstream execution. It SHALL
preserve actionable upstream HTTP statuses including 401, 422, 429, and 529, retain
a valid Retry-After header, and return sanitized structured errors without
credential or request-body content. It MUST NOT retry a failed evaluation
automatically on the same provider instance.

#### Scenario: Invalid question
- **WHEN** a Choice question lacks a criteria map or a Score question lacks a supported criteria array
- **THEN** the gateway returns HTTP 400 identifying the invalid field without evaluating the request

#### Scenario: Streaming or tools requested
- **WHEN** an evaluation request contains a stream or tools field
- **THEN** the gateway returns HTTP 400 with an unsupported-field error

#### Scenario: Upstream rate limit
- **WHEN** the final eligible TypeSafe attempt returns HTTP 429 with a valid Retry-After header
- **THEN** the client receives HTTP 429 and that header with a sanitized error, without a duplicate attempt to the same instance

#### Scenario: Invalid upstream response
- **WHEN** an upstream success response is malformed or lacks the required result fields
- **THEN** the gateway reports an upstream protocol error rather than a successful empty result
