# Design: OpenAI refresh-token rotation recovery

## Context

The OpenAI OAuth endpoint rotates refresh tokens. A successful exchange makes
the submitted token unusable. OmniLLM already collapses concurrent refreshes on
one provider object, but separate provider objects or processes can read the
same durable token and race. The loser receives a nested OpenAI error stating
that the refresh token was already used.

## Decisions

### Normalize token endpoint errors

The OpenAI adapter will decode successful token responses normally and decode
both the standard OAuth string fields and OpenAI's nested `error` object on
failure. Returned errors will contain the upstream type, code, and message but
will not embed the raw response body.

### Recover only from demonstrably newer durable state

After the upstream identifies a refresh token as already used, the provider
will read its durable credential record. It will retry once only when the
stored refresh token is non-empty and differs from the rejected token. This
allows a provider object or process that lost the rotation race to adopt the
winner's persisted token without creating an unbounded retry loop.

### Persist terminal reauthentication state

If durable state still contains the rejected token, the provider will clear
that refresh token in memory and in SQLite while retaining the existing access
token and account metadata. Subsequent requests will not repeatedly call the
token endpoint with a known-invalid credential. The returned error will direct
the operator to sign in again.

Persistence failure will be returned explicitly. OmniLLM will not claim the
invalid token was retired durably when that write fails.

## Compatibility and rollout

No schema migration is required because the existing provider token JSON can
represent an empty refresh token. Existing valid credentials are unchanged.
Standard OAuth error responses and successful responses remain supported.

## Failure handling

- A malformed success response remains a parse error without raw-body logging.
- A rotated-token rejection with a newer stored token gets one bounded retry.
- A retry failure is returned and is not recursively retried.
- A rotated-token rejection without newer state requires browser sign-in and
  disables automatic refresh attempts for that rejected token.
