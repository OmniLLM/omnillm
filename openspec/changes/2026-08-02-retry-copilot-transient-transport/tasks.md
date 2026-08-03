# Tasks

## 1. Classification

- [x] 1.1 Add `isTransientTransportFailure(err error, statusCode int) (bool, string)` to `internal/providers/copilot`, checking `isTimeoutError` first and returning false for timeouts.
- [x] 1.2 Cover connection-lost, connection-reset, unexpected-EOF, HTTP/2 `INTERNAL_ERROR`, and status 502/503/504.
- [x] 1.3 Unit-test the predicate against each transient case, each excluded case (timeout, context cancellation, 400, 401), and the reason string returned.

## 2. Backoff

- [x] 2.1 Add a jittered single-retry delay helper in the 150–400 ms range.
- [x] 2.2 Make the delay abort early on context cancellation.
- [x] 2.3 Unit-test that the delay stays within bounds and returns promptly on a canceled context.

## 3. Streaming path

- [x] 3.1 Wire the retry into `executeOpenAIStreamWithRetry` for the transport-error branch and the non-200 branch, before `parseOpenAISSE` is started.
- [x] 3.2 Ensure the retry is attempted at most once and does not interact with the existing auth retry or `/responses` fallback.
- [x] 3.3 Ensure the non-200 retry path closes the first response body before re-issuing.

## 4. Non-streaming path

- [x] 4.1 Wire the same single retry into `executeOpenAIWithRetry`.
- [x] 4.2 Confirm the request body is re-readable on the second attempt.

## 5. Diagnostics

- [x] 5.1 Emit one structured warning per retry with provider, endpoint, model, attempt number, and classification reason; assert no credentials or request content are included.
- [x] 5.2 Add `request_id`, `provider`, and `upstream_model` to the terminal error log in `internal/routes/route_logging.go`, threading the fields needed to do so.
- [x] 5.3 Confirm the client-disconnect branch still logs informationally and emits no provider-failure error.

## 6. Regression tests

- [x] 6.1 `httptest` server that drops the connection on attempt 1 and succeeds on attempt 2; assert a single uninterrupted stream and exactly two upstream attempts.
- [x] 6.2 `httptest` server returning 503 then 200; assert recovery for both streaming and non-streaming.
- [x] 6.3 Assert no retry occurs once a stream event has been emitted.
- [x] 6.4 Assert no retry occurs on timeout, preserving the existing single-attempt contract.
- [x] 6.5 Assert both-attempts-fail returns the final error to dispatch without extra attempts.

## 7. Verification

- [x] 7.1 `bun run spec:check`
- [x] 7.2 `make lint`
- [x] 7.3 `make typecheck`
- [x] 7.4 `make test`
