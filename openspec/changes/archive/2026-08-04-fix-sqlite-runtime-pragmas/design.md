## Context

See `proposal.md` for motivation. OmniLLM uses `modernc.org/sqlite`, but runtime and maintenance connection strings currently use parameter names associated with another SQLite driver. The driver ignores those keys, while the persistence specification assumes WAL, foreign keys, and a 5000 millisecond busy timeout are active. The runtime pool is intentionally limited to one connection, but independent processes and maintenance tools can still contend for the same database file.

## Goals / Non-Goals

**Goals:**

- Apply the existing SQLite policy using syntax supported by the pinned driver.
- Keep runtime and maintenance database openers consistent.
- Verify effective settings by querying SQLite rather than inspecting a connection string.
- Reproduce realistic cross-connection lock contention and prove metering persistence survives a transient lock.
- Preserve provider aggregate integrity by creating the provider-instance parent before authentication persists credentials or configuration.

**Non-Goals:**

- Add a general-purpose retry framework for every SQLite operation.
- Change the database schema, file location, migration sequence, or connection-pool size.
- Prevent multiple OmniLLM processes from opening the database.
- Change the asynchronous metering queue's overload/drop policy.
- Make token/config stores synthesize missing provider parents or weaken foreign-key enforcement.

## Decisions

### Use repeated `_pragma` parameters in SQLite DSNs

Construct connections with driver-supported parameters equivalent to:

```text
_pragma=journal_mode(WAL)
_pragma=busy_timeout(5000)
_pragma=foreign_keys(ON)
```

This directly fixes the mismatch with `modernc.org/sqlite` while retaining the existing policy values. Alternatives considered:

- Execute `PRAGMA` statements after `sql.Open`: rejected because pooled connections may not inherit connection-local settings consistently and maintenance openers could drift.
- Switch SQLite drivers: rejected because the behavior can be corrected without a dependency or portability change.

### Keep all direct database openers aligned

Update runtime initialization and maintenance scripts that construct their own DSNs. This avoids tools unexpectedly reverting to immediate lock failure or disabled foreign keys while operating on the same durable database.

A small shared DSN helper may be used if it reduces duplication without making standalone maintenance scripts harder to run. The implementation should follow existing package boundaries and avoid a broad persistence refactor.

### Test effective pragmas and real contention

Initialization tests will query `PRAGMA journal_mode`, `PRAGMA busy_timeout`, and `PRAGMA foreign_keys`. A contention test will use a second independent database handle to hold a conflicting lock, start a metering insert, release the lock before five seconds, and assert the insert completes successfully.

This checks behavior rather than string construction and fails under the currently ignored parameters. The test must use bounded synchronization and cleanup so a failure cannot hang the suite.

### Treat busy-timeout activation as the primary reliability fix

Do not add an application-level `SQLITE_BUSY` retry in this change. The specification already calls for SQLite to wait, and enabling the intended busy timeout addresses the observed immediate failure with less duplication and fewer compounded delays. A targeted retry can be proposed later if logs show lock failures exceeding the configured timeout.

### Reserve new provider parents at the orchestration boundary

Before a new provider's authentication method can persist credentials, configuration, model state, or cache rows, synchronously save its `provider_instances` record. If authentication, owned-record persistence, or registration fails, delete only the parent created by that attempt so cascading cleanup removes partial children. Existing-provider re-authentication must not delete a pre-existing parent on failure.

Keep this sequencing in provider-creation orchestration rather than synthesizing parents in child stores: token and configuration stores do not know the provider's canonical type, display name, or whether an absent parent indicates a caller bug. Registration may continue to refresh metadata asynchronously after the parent exists.

For providers whose canonical instance ID becomes known only after remote authentication, authenticate into memory first when possible, then reserve the canonical parent immediately before the first database child write. OpenAI OAuth reserves after successful code exchange and before `ApplyTokens`; GitHub Copilot reserves after deriving the canonical username-based ID and before `SaveToDB`.

Direct provider tests and server fixtures that intentionally call persistence primitives outside the creation route must seed the parent explicitly. Route-level creation tests remain end-to-end assertions of production ordering.

## Risks / Trade-offs

- **WAL activation can fail on unsupported filesystems** → Propagate initialization errors and keep tests on a local temporary filesystem.
- **Contention tests can become timing-sensitive** → Coordinate lock acquisition and release explicitly, use a release interval comfortably below five seconds, and bound the overall assertion.
- **A five-second wait can delay background writes during prolonged contention** → This is the existing normative policy; user requests remain decoupled through the asynchronous writer.
- **Maintenance scripts may not share runtime helpers** → Verify every direct DSN construction site identified by repository search and add a task to re-run that search.

## Migration Plan

1. Update supported pragma syntax in runtime and maintenance DSNs.
2. Run pragma and contention regression tests, then the full Go verification suite.
3. Deploy normally; existing database files require no schema or data migration. Opening the database activates WAL and the connection-local settings.
4. Roll back by reverting the DSN change; no stored-data rollback is required, though SQLite may leave normal WAL sidecar files until checkpointed.
