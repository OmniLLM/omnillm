# Design: Darwin managed lifecycle identity

## Context

Lifecycle state contains a PID, normalized executable path, and process start
identifier. Linux currently derives those values from `/proc/<pid>/exe` and
field 22 of `/proc/<pid>/stat`. The implementation is in a common source file,
so a Darwin binary compiles but fails every lifecycle registration at runtime.

The identity must distinguish PID reuse, avoid signaling unrelated processes,
and remain stable when the executable at the recorded path is atomically
replaced.

## Goals

- Support registration, validation, stop, and restart on macOS without CGO.
- Preserve the lifecycle state schema and Linux behavior.
- Preserve identity across atomic executable replacement.
- Keep unsupported-platform failure explicit and safe.

## Non-goals

- Changing the lifecycle state location or JSON schema.
- Changing background detachment, readiness, shutdown timing, or CLI output.
- Adding support for operating systems other than Linux and macOS.
- Changing gateway APIs, routing, persistence, CIF, or provider behavior.

## Decision

Move process identity collection behind build-tagged platform files:

- Linux continues reading `/proc/<pid>/exe` and `/proc/<pid>/stat`.
- Darwin reads `kern.proc.pid` through
  `golang.org/x/sys/unix.SysctlKinfoProc` for the process start timestamp and
  uses the native `PROC_PIDPATHINFO` process-information syscall for the
  kernel-recorded executable path.
- Other platforms return an explicit unsupported-platform error.

The Darwin start identifier will be a stable serialization of the kernel
process start timestamp with enough precision to distinguish PID reuse. The
executable path parser will validate the kernel response, extract the
kernel-recorded executable path, normalize symlinks when the target exists,
and retain the cleaned original path when an in-place replacement makes the
original file unavailable.

The common lifecycle registration, state comparison, atomic state-file claim,
ownership-safe cleanup, and signaling flow remain unchanged.

## Compatibility

The lifecycle JSON schema is unchanged. Existing Linux lifecycle files remain
readable. Darwin lifecycle files use the same fields, and all comparisons are
performed by binaries using the same native identity source.

Start IDs are platform-specific opaque strings; moving a lifecycle state file
between operating systems is unsupported and will safely be treated as stale.

## Failure handling

- Missing or inaccessible Darwin process metadata causes identity lookup to
  fail; existing state is treated as not running and is never used to signal a
  process.
- Malformed native process-path data returns a descriptive error rather than
  recording incomplete identity.
- Unsupported platforms fail registration before lifecycle state is written.
- Existing stale-state cleanup and bounded stop behavior remain unchanged.

## Verification

- Run common lifecycle tests on macOS.
- Add deterministic Darwin coverage for native identity, PID/start-ID
  mismatch rejection, executable replacement stability, and managed stop.
- Keep the Linux executable-replacement assertion Linux-only.
- Cross-compile lifecycle packages for Linux and Darwin.
- Run a native macOS build and an end-to-end background
  start/status/stop smoke test.
- Run the repository OpenSpec and relevant Go checks.
