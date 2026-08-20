## Context

GitHub default CodeQL currently runs Go 1.26.5 with `GOTOOLCHAIN=local`, while
the module requires Go 1.27. Default setup cannot be customized in the
repository to install the module toolchain.

## Decision

Use advanced CodeQL configuration in `.github/workflows/codeql.yml`. The Go
matrix entry uses manual build mode, `actions/setup-go` with `go.mod`, and
`go build ./...`; other supported languages use CodeQL's normal build mode.
Disable repository default setup after the workflow is pushed to prevent
duplicate and incompatible analysis.

The cleanup regression will assert absence of the removed command package,
`go run ./cmd/omniproxy`, and binary-table advertising rather than banning
every historical migration mention.

## Failure Handling

CodeQL and CI remain required merge signals. The PR is not merged until the
replacement checks finish successfully.
