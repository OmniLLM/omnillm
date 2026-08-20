# Change: Add CLI server lifecycle commands

## Why

The installed `omnillm` binary can start a gateway, but operators cannot use the
same binary to stop or restart it. Those operations exist only in the
repository's Bun development manager and Make targets, which are unavailable in
normal binary installations.

## What Changes

- Add `omnillm stop` and `omnillm restart` to the advertised Server command
  group.
- Track a running server instance in an operator-local PID file so a separate
  CLI invocation can request graceful shutdown.
- Make `restart` stop the tracked instance and then run the server with the same
  startup options accepted by `start`.
- Detect missing, stale, malformed, and mismatched lifecycle state and return
  actionable errors without signaling an unrelated process.
- Remove lifecycle state when the managed server exits normally or after a
  handled termination signal.

## Impact

- Affected capability: `cli-ops-config`.
- Affected code: root command registration, server lifecycle commands, server
  startup/shutdown integration, and focused command tests.
- Public CLI impact: two additive commands and an operator-local runtime-state
  file. Existing `omnillm start` foreground behavior remains unchanged.
- No API, provider, CIF, routing, persistence, dependency, or frontend behavior
  changes.
