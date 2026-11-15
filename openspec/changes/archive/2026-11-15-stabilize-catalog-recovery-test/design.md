## Context

The compatibility test re-registers a provider fixture. Registration deliberately rejects a provider with active lifecycle borrowers. Activation schedules asynchronous persistence that borrows providers and can overlap re-registration. The failure has occurred in CI and locally under repeated race runs.

## Goals / Non-Goals

**Goals:** Stabilize the fixture lifecycle while preserving all client compatibility assertions.

**Non-Goals:** Change provider leases, production persistence, runtime behavior, endpoints, or use blind sleeps to mask failures.

## Decisions

- Wait for already-scheduled provider persistence using the existing registry synchronization facility before fixture replacement. If investigation reveals other outstanding lifecycle operations, synchronize their actual completion in the test as well, rather than retrying or weakening production busy checks.
- Keep fixture-specific assertions and validate repeated race runs plus the complete relevant checks. Avoid fixed sleeps and indiscriminate retries because they hide timing issues.

## Risks / Trade-offs

- [Another borrower can outlive a response or scheduled save] → Diagnose remaining failures and add deterministic completion synchronization rather than assuming persistence is the only source.
- [Test coverage could be weakened] → Retain each client history and recovery assertion, and verify all subtests repeatedly.

## Migration Plan

The change is test-only; no runtime deployment is needed. Roll back the test change if it introduces unintended behavior, without modifying provider lease semantics.
