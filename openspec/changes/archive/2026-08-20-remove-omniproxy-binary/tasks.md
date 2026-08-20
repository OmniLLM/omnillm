## 1. Specification

- [x] 1.1 Strictly validate the proposal, delta specs, design, and tasks.
- [x] 1.2 Obtain human approval for all change artifacts before implementation.

## 2. Implementation

- [x] 2.1 Remove the `cmd/omniproxy` command package and proxy-specific ignore entries.
- [x] 2.2 Update Make and cross-platform installation scripts to build only `omnillm`.
- [x] 2.3 Rename the Tauri sidecar build output, bundle declaration, permissions, runtime lookup, and logging to `omnillm`.
- [x] 2.4 Update current English, Chinese, desktop, and architecture documentation to describe only `omnillm`.
- [x] 2.5 Make HTTP/2 keepalive verification independent of toolchain-specific TLS config mutation.
- [x] 2.6 Update all current module, container, CI-derived, OpenSpec, and documentation toolchain references to Go 1.27.

## 3. Verification

- [x] 3.1 Verify current non-archived sources contain no `omniproxy` references.
- [x] 3.2 Run `bun run spec:check`.
- [x] 3.3 Run `bun run lint:all`, `bun run typecheck`, `bun test`, and `bun run build`.
- [x] 3.4 Run `go vet ./...`, `go build ./...`, and `go test -race ./...` with the canonical Go 1.27 toolchain.
- [x] 3.5 Build the desktop sidecar and confirm its artifact is named `omnillm-<target-triple>`.

## 4. Archive

- [x] 4.1 Archive the validated change only after all tasks and verification pass.
