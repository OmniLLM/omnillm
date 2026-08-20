## Context

The current Makefile has more than thirty targets and maintains a dependency
stamp, lifecycle variables, formatted help, and wrappers around commands that
already have direct owners. `omnillm` now fully owns production lifecycle
management. Desktop sidecar naming remains the only genuinely cross-platform
orchestration that is awkward to express as one direct command.

## Goals and Non-Goals

Goals:

- Make the supported Make surface obvious at a glance.
- Establish `omnillm` as the only packaged server lifecycle interface.
- Preserve desktop build and development behavior on POSIX and Windows.

Non-goals:

- Remove Bun development scripts or the development manager.
- Change desktop runtime behavior.
- Change release or container behavior beyond removing Make aliases.

## Decisions

### Retain exactly three public Make targets

The Makefile retains `build-desktop-sidecar`, `build-desktop`, and
`desktop-dev`. General Go installation uses `go install .`; frontend and test
workflows use package scripts; server lifecycle uses `omnillm`; and Docker and
release operations use their direct commands.

### Use peer platform scripts for sidecar construction

Windows already uses `scripts/build-desktop-sidecar.ps1`. POSIX logic moves
from an inline Make macro to `scripts/build-desktop-sidecar.sh`. Make selects
one script and sequences Tauri commands, avoiding embedded platform logic.

### Remove generated help

With three descriptive target names, a large help renderer would be larger
than the build logic. Current documentation lists the retained targets and
their direct-command alternatives.

## Compatibility

Removed targets fail with Make's normal “No rule to make target” diagnostic.
Callers migrate to the documented direct command. Package scripts for desktop
build and development continue to work because their Make targets remain.
