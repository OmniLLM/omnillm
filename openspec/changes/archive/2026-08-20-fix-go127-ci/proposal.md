## Why

PR verification exposed two integration gaps in the archived Go 1.27 and
`omniproxy` cleanup work. GitHub default CodeQL uses Go 1.26 with toolchain
downloads disabled, so Go analysis cannot build the Go 1.27 module. The cleanup
regression also rejects a migration note about removing a stale executable,
even though that note does not advertise the removed command.

## What Changes

- Add an advanced CodeQL workflow that resolves Go from `go.mod` and manually
  builds Go code for analysis.
- Replace GitHub default CodeQL setup with the repository-owned workflow.
- Narrow cleanup regression coverage to prohibited entrypoint and invocation
  documentation while allowing the uninstall migration note.

## Capabilities

- `spec-governance`: keep security analysis compatible with the canonical Go
  toolchain and make cleanup regression coverage match the normative contract.

## Impact

Runtime behavior is unchanged. CI continues analyzing Actions, Go,
JavaScript/TypeScript, and Rust, with Go 1.27 sourced from `go.mod`.
