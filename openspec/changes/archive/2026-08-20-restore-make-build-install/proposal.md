## Why

The minimized Makefile intentionally removed all generic Go wrappers, but the
operator has approved three narrowly scoped conveniences: building the
canonical `omnillm` artifact, installing it through the Go toolchain, and
removing that installed executable. These operations do not duplicate the
self-contained server lifecycle.

## What Changes

- Add `build` to compile `omnillm` into `.build/bin`.
- Add `install` to run `go install .` using Go's configured installation path.
- Add `uninstall` to remove only `omnillm` and any stale legacy `omniproxy`
  executable from Go's effective binary installation directory.
- Keep every lifecycle, Bun, validation, release, Docker, and help alias absent.
- Update current documentation and Makefile regression coverage for the
  six-target surface.

## Capabilities

- `cli-ops-config`: permit `build`, `install`, and `uninstall` alongside the
  three desktop orchestration targets.

## Impact

This adds three convenience targets without changing runtime behavior or the
canonical `omnillm` lifecycle interface.
