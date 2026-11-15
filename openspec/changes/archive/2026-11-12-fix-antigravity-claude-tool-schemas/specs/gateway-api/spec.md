## ADDED Requirements

### Requirement: Provider-qualified non-streaming fallback eligibility
The gateway SHALL honor a provider error's explicit non-streaming fallback eligibility even when the error is wrapped. Antigravity upstream HTTP 4xx failures SHALL disallow an immediate replay through the collected-response path. Existing bounded provider-internal authentication recovery, other providers' default fallback eligibility, and client-facing error envelopes SHALL remain unchanged.

#### Scenario: Invalid Antigravity tool schema
- **WHEN** a streaming Messages, Chat Completions, or Responses request receives an Antigravity HTTP 400 before output starts
- **THEN** the gateway reports the failure without issuing the same request again as non-streaming

#### Scenario: Other Antigravity HTTP client errors
- **WHEN** Antigravity returns another HTTP 4xx after any applicable provider-internal authentication recovery
- **THEN** the gateway does not repeat that failure through its non-streaming fallback

#### Scenario: Existing fallback remains available
- **WHEN** an error has no explicit fallback restriction or an Antigravity HTTP 5xx retains existing fallback eligibility
- **THEN** the gateway applies its existing cancellation, request-policy, authentication, and fallback rules
