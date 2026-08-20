## Why

The Makefile still duplicates lifecycle, development, validation, release, and
container commands that already have canonical direct entrypoints. In
particular, `omnillm` now owns its own start, stop, restart, status, and log
operations, so retaining Bun-backed Make lifecycle wrappers creates a second
operational path with different options and behavior.

## What Changes

- Reduce the Makefile to desktop sidecar orchestration only:
  `build-desktop-sidecar`, `build-desktop`, and `desktop-dev`.
- Move the POSIX desktop sidecar build recipe into a peer shell script so the
  Makefile only dispatches to the platform-specific script.
- Remove Make aliases for install/build, dependencies, frontend development,
  server lifecycle, tests, linting, releases, Docker, and help.
- Update current documentation to use `omnillm` for lifecycle operations and
  direct Go, Bun, and Docker commands for their respective workflows.

## Capabilities

- `cli-ops-config`: make `omnillm` the sole lifecycle interface and constrain
  Make to desktop packaging orchestration.
- `desktop-app`: preserve the existing desktop build and development workflows
  through the three retained Make targets.

## Impact

This removes convenience Make aliases and is intentionally incompatible with
scripts that invoke those aliases. The underlying `omnillm`, Go, Bun, release,
and Docker commands remain available directly. Gateway and desktop runtime
behavior do not change.
