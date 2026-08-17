## MODIFIED Requirements

### Requirement: Prompt-cache mechanism separation
Provider prompt caching SHALL remain separate from channel affinity and the exact-response cache: prompt-cache directives SHALL NOT enable, disable, read, write, or change the semantic identity of the exact-response cache; a provider prompt-cache hit SHALL still execute upstream inference unless an independently enabled exact-response entry is served; and an exact-response hit SHALL NOT be reported as provider prompt-cache activity.

#### Scenario: Prompt control without response-cache control
- **WHEN** a request contains JSON prompt-cache directives but no exact-response cache instruction
- **THEN** prompt-cache processing occurs without changing response-cache enablement or per-request response-cache behavior

#### Scenario: Semantically identical prompt-cache placement
- **WHEN** two otherwise semantically identical requests differ only in prompt-cache breakpoint placement, native cache key, or retention hints
- **THEN** they share exact-response semantic identity while retaining their distinct provider controls on an exact-response miss

#### Scenario: Provider cache hit after exact-response miss
- **WHEN** no exact-response entry is served and the upstream provider reports cached input
- **THEN** the request is recorded as provider prompt-cache activity from an upstream execution

#### Scenario: Exact-response hit
- **WHEN** the gateway serves an exact-response cache entry without upstream execution
- **THEN** it does not infer or report a new provider prompt-cache hit, miss, read, or write
