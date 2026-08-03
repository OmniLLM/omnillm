# Tasks

## 1. Non-mutating trace body truncation

- [x] 1.1 Rewrite `cappedBody` in `internal/providers/openaicompat/http.go` to allocate a new buffer instead of appending onto a reslice of the caller's array.
- [x] 1.2 Add a regression test proving the source buffer is byte-identical after truncation, using a slice with spare capacity so the aliasing condition is reproduced.
- [x] 1.3 Add a test proving repeated truncation of the same buffer yields identical results.
- [x] 1.4 Add a test proving a payload at or below the limit is returned unchanged.

## 2. Rune-aware truncation

- [x] 2.1 Make `cappedBody` cut at a UTF-8 boundary at or below the byte limit.
- [x] 2.2 Make `truncateToolLoopValue` in `internal/routes/tool_loop_logging.go` cut at a UTF-8 boundary at or below the byte limit.
- [x] 2.3 Preserve the existing `...(truncated)` marker and existing limits in both helpers.

## 3. Encoding regression tests

- [x] 3.1 Test `truncateToolLoopValue` with multi-byte input straddling the 400-byte boundary; assert `utf8.ValidString` on the result.
- [x] 3.2 Test the same for `cappedBody` at its 1024-byte boundary; assert `utf8.Valid`.
- [x] 3.3 Test ASCII inputs still truncate exactly at the limit with the marker.
- [x] 3.4 Test values within the limit pass through unchanged with no marker.
- [x] 3.5 Sweep every boundary offset across a multi-byte string to prove the result is valid UTF-8 and never exceeds the byte limit.

## 4. Verification

- [x] 4.1 `bun run spec:check`
- [x] 4.2 `make lint`
- [x] 4.3 `make typecheck`
- [x] 4.4 `make test` and `go test ./...`
