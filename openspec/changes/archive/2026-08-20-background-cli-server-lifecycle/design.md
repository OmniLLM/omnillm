# Design: Background CLI-managed server processes

## Context

The binary now has safe lifecycle identity and graceful stop support, but
`start` directly calls the blocking server runner. Backgrounding must avoid a
recursive public-command launch, accurately report bind/configuration failures,
detach from the terminal, and retain a foreground mode for supervisors.

## Decisions

### Parent and child roles

The public `start` command validates flags and launches the current executable
through a hidden internal child entrypoint. The child receives the resolved
startup configuration, performs normal initialization, binds the listener, and
owns lifecycle registration and cleanup. The hidden entrypoint is not shown in
help and is not a separately supported operator interface.

`--foreground` bypasses child spawning and runs the same server runner directly.
This keeps one server implementation and provides an explicit attached mode.

### Readiness handshake

The parent and child use an inherited, one-shot readiness channel. The child
reports success only after listener binding and lifecycle registration. It
reports a sanitized startup error when initialization fails first. The parent
waits for either readiness, child exit, or a bounded timeout. On success it
prints the managed PID/address and exits zero; on failure it reaps the child and
returns a single actionable error.

### Detachment and logging

Platform-specific helpers configure the child as an independent process group
without relying on shell syntax. Standard input is disconnected. Standard
output and error append to the existing operator-local OmniLLM log, with file
permissions restricted to the operator. Secret-bearing configuration is passed
through inherited environment or protected inherited data rather than persisted
in lifecycle state or displayed command arguments.

### Restart composition

`restart` retains the existing validated graceful-stop phase, then invokes the
same background start orchestration. With `--foreground`, the replacement stays
attached. If no managed server exists, restart continues directly to startup.

## Failure handling

- Initialization or bind failure: child reports the error, parent returns
  nonzero, and no live lifecycle state remains.
- Readiness timeout: parent terminates and reaps the child and returns nonzero.
- Parent interruption during startup: parent terminates and reaps the child.
- Log open failure: startup fails before spawning.
- Stop failure during restart: replacement is not launched.

## Compatibility

The default attachment behavior changes intentionally. Existing automation that
executes `omnillm start` under Docker, systemd, launchd, Kubernetes, or another
supervisor can use `omnillm start --foreground`. All existing startup flags and
environment precedence remain available to both modes.

## Testing

Deterministic tests cover argument/config forwarding, hidden entrypoint
discovery, foreground selection, readiness success, early failure, timeout,
cleanup, and restart composition. Isolated binary smoke tests verify that both
public commands return while their server remains reachable and stoppable.
