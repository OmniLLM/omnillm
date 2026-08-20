## Context

`main.go` and `cmd/omniproxy/main.go` currently register equivalent CLI
surfaces under different executable names. General install scripts build both,
while the Tauri desktop app specifically builds, permits, bundles, and launches
the proxy-branded executable as its backend sidecar.

## Goals and Non-Goals

Goals:

- Leave one Go command entrypoint and executable name: `omnillm`.
- Keep the desktop application self-contained by packaging the canonical
  `omnillm` executable as its sidecar.
- Remove current build, packaging, and documentation references that would
  recreate or advertise `omniproxy`.
- Align every current Go toolchain pin and description on Go 1.27.

Non-goals:

- Change the CLI command surface or gateway API behavior.
- Delete already-installed binaries from operator machines.
- Rewrite archived historical records that describe the repository at an
  earlier point in time.

## Decisions

### Use `omnillm` as the desktop sidecar name

The desktop app will build the root Go package into Tauri's target-triple
sidecar naming convention as `binaries/omnillm-<target-triple>`. Tauri config,
shell permissions, Rust lookup calls, and log targets will all use `omnillm`.
This retains the existing sidecar lifecycle while eliminating the second Go
entrypoint.

### Remove rather than redirect `omniproxy`

No compatibility shim, symlink, wrapper, or alias will remain because the
requested end state is a single binary. Attempts to invoke `omniproxy` after
upgrading will fail through normal executable lookup, and users must migrate
to `omnillm`.

### Preserve historical archives

Files under `docs/archive` and `openspec/changes/archive` remain unchanged.
They are historical evidence rather than current build or usage guidance.

### Use Go 1.27 as the canonical toolchain

The module `go` directive is the CI source of truth because GitHub Actions
already uses `go-version-file: go.mod`. The Docker builder image, OpenSpec
project context, and current English and Chinese documentation will carry the
same version. Archived records remain unchanged because they describe earlier
states.

### Verify HTTP/2 settings through the configured transport

Go 1.27 no longer guarantees that `http2.ConfigureTransports` mutates
`http.Transport.TLSClientConfig`, so the earlier test depended on a toolchain
implementation detail. The transport constructor exposes the configured
HTTP/2 transport to same-package tests, which assert the actual read-idle and
ping timeouts. Public callers continue receiving the same `*http.Transport`;
runtime settings are unchanged.

## Compatibility and Rollout

The release notes and current README will call out the removal. Automation,
service definitions, or shell scripts that invoke `omniproxy` must switch to
`omnillm`. Desktop users require no manual migration because the packaged app
will launch the renamed sidecar internally. Build environments must provide Go
1.27 or allow Go's toolchain selection to obtain it.

## Failure Handling

Desktop sidecar lookup and startup errors continue through the existing error
path. Renaming is coordinated across build output, bundle declaration,
capability allowlists, and runtime lookup so partial configuration is caught by
desktop build and startup verification.
