## Why

OmniLLM's SQLite connection string uses parameter names from a different Go driver, so `modernc.org/sqlite` silently ignores the intended WAL, 5000 millisecond busy-timeout, and foreign-key settings. Runtime logs show a metering record being lost to an immediate `SQLITE_BUSY`, contradicting the existing persistence contract and making concurrent processes or maintenance tools unsafe.

## What Changes

- Configure SQLite through `modernc.org/sqlite`'s supported `_pragma` connection parameters so WAL, the 5000 millisecond busy timeout, and foreign keys are active on runtime connections.
- Update maintenance scripts that directly open OmniLLM's database to use the same supported pragma syntax.
- Add initialization tests that query the effective SQLite pragmas rather than assuming DSN parameters were applied.
- Add a second-connection contention test proving a metering insert waits for a transient lock and succeeds when the lock is released within the busy-timeout window.
- Make new-provider persistence parent-first so effective foreign-key enforcement does not reject credential and configuration rows.
- Add provider-creation regression coverage and update direct persistence fixtures to satisfy the enforced ownership contract.
- Preserve the existing one-open/idle-connection pool policy and asynchronous metering behavior.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `persistence`: Strengthen verification of the existing SQLite connection policy and concurrent-write behavior so the configured pragmas must be effective on actual database connections; enforce parent-first provider aggregate persistence when foreign keys are active.

## Impact

- Affected runtime code: `internal/database/init.go`, provider-creation orchestration, and database/provider/route tests.
- Affected maintenance tools: scripts that construct their own SQLite DSNs.
- Observable impact: concurrent external database access waits for up to five seconds instead of causing immediate lock errors; foreign-key enforcement and WAL are reliably enabled.
- No public API, schema, dependency, or configuration-format changes.
