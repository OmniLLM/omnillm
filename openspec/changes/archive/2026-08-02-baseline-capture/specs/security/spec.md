## Purpose

Defines OmniLLM trust boundaries for inbound authentication, endpoint SSRF controls, public route exceptions, CORS, concurrency, configuration-file access, and safe default binding.

## ADDED Requirements

### Requirement: Optional inbound authentication
Protected routes SHALL enforce credentials when a master API key is configured and SHALL permit open mode when it is empty.

#### Scenario: Key configured
- **WHEN** a protected request has no accepted credential
- **THEN** the gateway returns HTTP 401 with `{"error":"unauthorized"}`

### Requirement: Credential presentation
The gateway SHALL accept a case-insensitive two-part bearer header or `x-api-key`, SHALL prefer a valid bearer token, and SHALL never accept query-string credentials.

#### Scenario: SSE query key
- **WHEN** an event-stream request supplies only an `api_key` query parameter
- **THEN** it is rejected to prevent credential leakage in logs

### Requirement: Constant-time master-key comparison
The master key SHALL be compared in constant time before access-token fallback.

#### Scenario: Master key mismatch
- **WHEN** an equal-length presented key differs
- **THEN** comparison does not short-circuit and authentication proceeds to access-token lookup

### Requirement: Hashed access-token fallback
Non-master credentials SHALL be SHA-256 hashed and authorized only when an enabled, unexpired stored token matches, with token identity attached to request context.

#### Scenario: Valid access token
- **WHEN** the presented token hash matches a valid record
- **THEN** the request is authorized and token ID and name become available for attribution

### Requirement: Public route exceptions
Health endpoints, public server information, and required Antigravity/OpenAI OAuth callback or status routes SHALL remain unauthenticated.

#### Scenario: OAuth callback
- **WHEN** an identity provider redirects without an authorization header
- **THEN** the configured public callback processes the request

### Requirement: Provider endpoint SSRF validation
Operator-supplied endpoints SHALL use HTTP or HTTPS with a host and SHALL reject literal loopback, private, and link-local destinations unless local endpoints are explicitly allowed.

#### Scenario: Metadata endpoint
- **WHEN** `169.254.169.254` is supplied without local opt-in
- **THEN** validation rejects it before storage or network access

#### Scenario: Local opt-in
- **WHEN** local endpoints are explicitly enabled
- **THEN** localhost may pass while scheme and host validation still apply

### Requirement: Restricted browser origins
Cross-origin access SHALL allow loopback origins and Chrome extensions, optionally restricting extension IDs to a configured nonblank allowlist.

#### Scenario: Remote web origin
- **WHEN** a non-loopback HTTP origin is presented
- **THEN** it is not allowed

### Requirement: Authenticated concurrency ordering
Proxy concurrency limiting SHALL run after authentication so rejected unauthenticated traffic cannot occupy capacity.

#### Scenario: Invalid credential under load
- **WHEN** authentication rejects a request
- **THEN** no concurrency slot is acquired

### Requirement: Privileged configuration editing
Writing or importing external tool configuration SHALL be disabled by default, limited to known target names, validate JSON-backed formats, and write owner-only files.

#### Scenario: Caller-supplied path
- **WHEN** a configuration operation attempts to identify an arbitrary path
- **THEN** the destination is resolved only from the server's fixed known-name map

### Requirement: Safe default network binding
The server SHALL default to binding `127.0.0.1` unless an operator explicitly selects another host.

#### Scenario: Host omitted
- **WHEN** the server starts without a host flag
- **THEN** it listens only on IPv4 loopback by default
