# Design: Binary server lifecycle commands

## Context

`omnillm start` calls the Go server directly and blocks in the foreground. The
repository-only Bun manager separately implements background process tracking,
but installed binary users do not have that manager. A stop command therefore
needs durable, per-operator identity for the foreground server process.

## Decisions

### Preserve foreground startup

`start` remains a foreground command. This preserves shell, container,
supervisor, and service-manager expectations. A user may run it under their
preferred supervisor or in one terminal and invoke `stop` from another.

### Store validated lifecycle identity

After the listener is successfully established, the server atomically writes a
small lifecycle-state file below OmniLLM's existing operator configuration
directory. The state includes the PID and process identity data that can be
validated before signaling. Startup refuses to overwrite state for another live
OmniLLM instance; malformed or stale state is cleaned up safely.

Lifecycle-file creation and removal are owned by a narrow lifecycle component.
Focused tests use isolated subprocesses and temporary state paths. Cleanup only
removes state still owned by the current process, preventing an exiting old
process from deleting a newer server's state.

### Graceful bounded stop

`stop` loads and validates the lifecycle identity, sends the platform's graceful
termination signal, and polls for exit for a bounded duration. It reports a
timeout instead of escalating to an unconditional kill. Invalid identity never
causes a signal to be sent.

### Restart composes stop and start

`restart` accepts the same server startup flags as `start`. If a valid managed
server exists, it performs the same graceful bounded stop; if none exists, it
continues as a start. It then runs the replacement in the foreground. Startup
configuration is explicitly supplied to `restart`; secrets and previous argv
are not persisted in the lifecycle file.

## Compatibility

The change adds commands without removing or renaming existing ones. `start`
continues to block and retains its current flags and environment precedence.
The Bun manager and Make lifecycle targets remain independent and unchanged;
their processes are not adopted unless they produce the validated binary
lifecycle state.

## Failure handling

- Missing state: `stop` reports that no managed server is running; `restart`
  proceeds to startup.
- Malformed or stale state: remove it when safe, never signal its PID, and
  report the condition for `stop`.
- Identity mismatch: treat the state as stale and do not signal the process.
- Graceful-stop timeout: return an error and leave enough state for diagnosis or
  a later retry.
- Startup failure: do not leave lifecycle state claiming the server is live.

## Testing

Focused tests will use temporary state paths and isolated subprocesses to
cover registration, ownership-safe cleanup, valid stop, stale/malformed state,
identity mismatch, timeout, restart-with-process, and restart-without-process.
Root help tests will verify that both commands are advertised in the Server
group. Existing server and full Go verification will guard startup behavior.
