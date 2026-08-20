## Why

The repository currently builds and documents two Go command binaries,
`omnillm` and `omniproxy`, even though both expose the same gateway command
surface. Maintaining the duplicate proxy-branded entrypoint also makes desktop
packaging and installation workflows carry a second binary name unnecessarily.

## What Changes

- Remove the `cmd/omniproxy` Go entrypoint and stop building or installing an
  `omniproxy` executable.
- Make repository build and installation workflows produce only `omnillm`.
- Package and launch `omnillm` as the Tauri desktop backend sidecar.
- Update current documentation, ignore rules, desktop permissions, and build
  scripts to use only the `omnillm` binary name.
- Make Go 1.27, the latest installed Go release, the canonical toolchain in the
  module directive, container build, CI (through `go.mod`), OpenSpec context,
  and current documentation.
- Keep HTTP/2 keepalive verification compatible with Go 1.27 without changing
  runtime transport settings.
- Preserve archived historical design and verification records as immutable
  history.

## Capabilities

- `cli-ops-config`: define `omnillm` as the repository's sole supported Go
  command binary.
- `desktop-app`: change the packaged backend sidecar identity from
  `omniproxy` to `omnillm` without changing its runtime contract.

## Impact

This is an intentional compatibility break for users invoking `omniproxy`.
They must invoke `omnillm` instead. Contributors and builders must use Go 1.27
or newer. Gateway APIs, command behavior, storage, and provider behavior are
unchanged; the HTTP/2 test refactor has no runtime behavior impact. Existing
`omniproxy` files previously installed outside the repository are not deleted
by builds or installers.
