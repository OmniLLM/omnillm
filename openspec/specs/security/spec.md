# security Specification

## Purpose
Defines OmniLLM trust boundaries for inbound authentication, endpoint SSRF controls, public route exceptions, CORS, concurrency, configuration-file access, and safe default binding.
## Requirements
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

### Requirement: Sensitive payload exclusion from logs
OmniLLM SHALL NOT emit tool argument content, tool result content, or credential material derived from those payloads into structured logs at any enabled log level or destination. This exclusion SHALL apply whether the payload is plain text, structured data, streamed in deltas, marked as an error, or longer than a diagnostic size limit.

#### Scenario: Secret-bearing tool arguments
- **WHEN** a tool call contains a synthetic credential sentinel in nested or streamed arguments
- **THEN** no structured log record contains the sentinel or any argument-content preview

#### Scenario: Secret-bearing tool result
- **WHEN** a client returns a plain-text or structured tool result containing a synthetic credential sentinel
- **THEN** no structured log record contains the sentinel or any result-content preview

#### Scenario: Errored Agent tool result at default level
- **WHEN** an Agent tool result is marked as an execution error and warning logs are enabled
- **THEN** the warning contains no tool result content or credential material

### Requirement: Secure OAuth authorization material
OAuth authorization state and PKCE material SHALL be generated from a cryptographically secure random source with provider-compatible entropy and encoding, and the system SHALL NOT issue an authorization URL when secure generation fails.

#### Scenario: OpenAI authorization material
- **WHEN** an OpenAI OAuth flow starts successfully
- **THEN** it uses a 32-byte unpadded base64url PKCE verifier with an S256 challenge and a 16-byte unpadded base64url state value

#### Scenario: Antigravity authorization state
- **WHEN** an Antigravity OAuth flow starts successfully
- **THEN** it uses a 16-byte state value encoded as 32 lowercase hexadecimal characters

#### Scenario: Secure random generation fails
- **WHEN** secure random generation fails while an OAuth flow is starting
- **THEN** flow initiation fails without storing pending state or returning an authorization URL

### Requirement: OAuth callback state validation
An OAuth authorization code SHALL be exchanged only after the callback state is present, valid for the pending flow, unexpired where expiry is tracked, and consumed according to the provider flow's existing lifecycle.

#### Scenario: Missing or mismatched state
- **WHEN** an OAuth callback omits state or presents a value that does not match a pending flow
- **THEN** the callback is rejected and its authorization code is not exchanged

#### Scenario: Expired Antigravity state
- **WHEN** an Antigravity callback presents an expired pending state
- **THEN** the callback is rejected and its authorization code is not exchanged

#### Scenario: Valid Antigravity state
- **WHEN** an Antigravity callback presents a valid pending state
- **THEN** the state is consumed before the authorization code exchange begins

