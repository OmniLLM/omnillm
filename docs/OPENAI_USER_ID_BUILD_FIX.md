# OpenAI User ID Build Fix

## Context

The Go backend stopped rebuilding after the OpenAI-compatible Responses serializer started calling `shared.TruncateOpenAIUserID`, but the shared helper was never added to `internal/providers/shared`.

## What Changed

### Before

- `internal/providers/openaicompat/responses.go` referenced `shared.TruncateOpenAIUserID`
- `internal/providers/shared` did not define that symbol
- any backend rebuild failed at compile time inside `internal/providers/openaicompat`

### After

- added `shared.TruncateOpenAIUserID` to trim and cap OpenAI-compatible `user` identifiers
- applied the same sanitizer in OpenAI-compatible chat serialization
- re-applied truncation after chat `Extras` merging so an overridden `user` field cannot bypass the limit
- added regression coverage for the helper and chat serialization path

## Why This Is Critical

This was a backend build-break in the shared OpenAI-compatible provider layer:

- local rebuilds failed immediately
- provider changes under `openaicompat` could not be tested or shipped
- both chat-completions and responses paths risked drifting on `user` handling

## Affected Files

- `internal/providers/shared/helpers.go`
- `internal/providers/shared/helpers_test.go`
- `internal/providers/openaicompat/serialization.go`
- `internal/providers/openaicompat/serialization_test.go`

## Verification

- Added unit coverage for trimming and truncating oversized OpenAI `user` values
- Verified `go test ./internal/providers/shared ./internal/providers/openaicompat`

## Commit Range

- Working tree changes after current `HEAD` (uncommitted fix set)
