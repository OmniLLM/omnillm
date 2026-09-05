# Change: Support managed server lifecycle on macOS

## Why

The `omnillm start`, `stop`, and `restart` lifecycle currently reads Linux
`/proc` metadata unconditionally. On macOS, registration fails before the
server can report readiness because `/proc/<pid>/exe` and `/proc/<pid>/stat`
do not exist.

## What Changes

- Add Darwin-native managed-process identity collection using macOS sysctl and
  process-information metadata.
- Preserve the existing lifecycle guarantees that reject PID reuse and
  unrelated processes.
- Preserve lifecycle identity when the managed executable is atomically
  replaced while the server remains running.
- Keep Linux lifecycle behavior unchanged and return an explicit unsupported
  platform error where no process-identity implementation exists.
- Add platform-specific deterministic lifecycle coverage and verify a native
  macOS start/status/stop flow.

## Capabilities

- `cli-ops-config`: modify managed server lifecycle requirements to require
  native Linux and macOS support.

## Runtime Impact

This change enables managed foreground and background server lifecycle
operations on macOS. Linux behavior remains unchanged. Other unsupported
operating systems receive an explicit process-identity error instead of an
implicit `/proc` filesystem failure.
