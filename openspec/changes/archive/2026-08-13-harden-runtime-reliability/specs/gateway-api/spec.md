## ADDED Requirements

### Requirement: Bounded inbound request bodies
The gateway SHALL enforce a finite request-body limit for Chat Completions, Responses, Messages, and Messages token-counting requests and SHALL return HTTP 413 with the dialect's structured invalid-request error when the limit is exceeded.

#### Scenario: Generation request exceeds the limit
- **WHEN** a client posts a generation request body larger than the configured gateway limit
- **THEN** the gateway returns HTTP 413 without parsing or dispatching the request

#### Scenario: Token-count request exceeds the limit
- **WHEN** a client posts a Messages token-counting body larger than the configured gateway limit
- **THEN** the gateway returns HTTP 413 without attempting local token estimation

#### Scenario: Request remains within the limit
- **WHEN** a syntactically valid request body is at or below the configured gateway limit
- **THEN** existing parsing, routing, and response behavior remains unchanged

### Requirement: Cancellable rate-limit waiting
A gateway request waiting for an available rate-limit time SHALL observe its request context and SHALL stop waiting promptly when the client cancels or the server cancels the request.

#### Scenario: Client cancels queued request
- **WHEN** a request context is canceled while waiting for a rate-limit reservation
- **THEN** the wait ends promptly and the canceled reservation does not delay later active requests

#### Scenario: Active queued request reaches reservation
- **WHEN** a queued request remains active until its reserved time
- **THEN** it proceeds under the configured request-spacing contract
