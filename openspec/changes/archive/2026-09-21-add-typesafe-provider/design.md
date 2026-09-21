# TypeSafe provider design

## Context and verified upstream contract

Sources: https://docs.typesafe.ai/api.md and https://docs.typesafe.ai/models.md.
The API uses bearer authentication at `https://api.typesafe.ai`. Native evaluation
is `POST /v1/systemone`, with `model`, `state`, and a map of `questions`. Answers
are typed Noul probabilities, Choice selections/distributions/confidence, or Score
values/legends/distributions/confidence. Instructions and criteria can contain
structured JSON. Input is text or structured textual state, not multimodal input.

Authenticated `GET /v1/models` returns `{models: [{name, description,
release_date}]}` rather than the OpenAI model-list envelope. A read-only check
using the supplied credential returned HTTP 200 with `jev-latest` and
`jev-preview`. The docs also allow explicit version identifiers not listed by
discovery. No evaluation has been sent during proposal preparation.

## Decisions

1. **Expose the native operation.** Add `/v1/systemone` and a small optional
   context-aware evaluation interface implemented by the TypeSafe provider.
   Preserve JSON through request/response types appropriate for the upstream
   contract; do not force state and questions into CIF chat messages. Existing
   Provider legacy methods and its generation adapter must reject unsupported
   operations explicitly, including direct calls that bypass routing guards.
2. **Integrate the full provider lifecycle.** Add the provider ID and factory
   cases, persisted key/config restoration, authentication and re-authentication,
   model listing/refresh, CLI setup, and frontend setup. Reuse existing stores and
   registration rollback rules. Verify credentials with `/v1/models`, avoiding a
   paid evaluation during normal authentication. Use the fixed official HTTPS
   endpoint; custom endpoints are outside this change.
3. **Keep capabilities accurate.** Add explicit evaluation metadata and negative
   generation/tool/streaming/embedding capabilities. Capability filtering must
   preserve the behavior of existing providers that have incomplete metadata;
   only explicit evaluation-only providers lose generation eligibility. Extend
   the compatibility manifest without pretending Jev passes generation tests.
4. **Reuse routing policy.** Share instance/alias/name and virtual-model
   resolution, active status, priority, and model enablement. Filter candidates
   for the requested operation before execution. Explicit provider-qualified
   version identifiers are forwarded unchanged. Preserve existing ordering and
   retryable-failure classification between distinct eligible candidates; no
   same-instance automatic retries in the initial implementation. Preserve a
   final upstream Retry-After header so clients can implement bounded backoff.
5. **Use existing trust boundaries.** Register the route behind proxy auth and
   concurrency controls. Apply a finite body limit consistent with existing
   proxy handling, reject invalid schema before dispatch, and bound upstream
   requests with caller context and a finite timeout. Return structured sanitized
   errors, preserving relevant upstream status without copying upstream bodies
   that may contain secrets or request content. Do not follow credential-bearing
   redirects to a different origin.
6. **Record native usage.** Reuse existing metering fields with API shape
   `systemone`, mapping input/output token counts directly. No generation CIF,
   prompt-cache claims, exact-response caching, or new persistence schema is
   required. Validate metering filters against the added API shape.
7. **Keep credentials operator-controlled.** CLI `--api-key` takes precedence
   over `TYPESAFE_API_KEY`; otherwise use the established secure prompt. The
   product never sources `~/.config/typesafe/env`. Local verification may use
   `set -a; source ~/.config/typesafe/env; set +a` without shell tracing, then pass
   credentials through the environment or in-memory HTTP headers. Never place
   the key in command arguments, fixtures, reports, or committed configuration.

## Public request example

```json
{
  "model": "jev-latest",
  "state": {"message": "Please refund the duplicate charge."},
  "questions": {
    "refund": {"type": "noul", "instructions": "Is a refund requested?"},
    "team": {
      "type": "choice",
      "instructions": "Which team should handle this?",
      "criteria": {"billing": "Payment issues", "technical": "Software faults"}
    },
    "urgency": {
      "type": "score",
      "instructions": "How urgent is this request?",
      "criteria": ["Routine", "Time-sensitive", "Immediate action needed"]
    }
  }
}
```

An operator can use `<instance-or-alias>/jev-latest` to select a particular
TypeSafe account. Native response fields remain intact; the response model can be
the resolved version even if the request used an alias.

## Alternatives considered

- A generic OpenAI-compatible entry would call unsupported chat endpoints and
  misread TypeSafe's model-list envelope.
- An automatic chat-to-question conversion would require inventing criteria or
  discarding user intent and would imply unsupported coding-agent compatibility.
- Passing JSON inside a chat message would introduce an arbitrary wrapper and
  complicate all three generation dialects. Native evaluation is a smaller,
  documented contract for existing TypeSafe SDK and HTTP clients.

## Failure handling and validation

Validate native question types and structured values according to the current
HTTP reference, including the documented Choice maximum of 255 options and Score
range of 2–10 levels. Reject unknown top-level fields instead of silently dropping
generation parameters. Upstream semantic validation remains authoritative for
model/context limits. Check successful response envelopes and required fields
without rounding probabilities or discarding native answer detail.

Test upstream authentication failure, validation errors, rate limit/overload,
malformed responses, cancellation, and timeout. Preserve previous credentials on
failed re-authentication and successful cached catalogs on discovery failure.

## Rollout and verification

This is additive and requires no migration. Registration is explicit; credentials
are never auto-imported at startup. After implementation, run provider, routing,
admin, CLI, frontend, and client compatibility tests plus the standard Bun/Go
checks. Use temporary runtime storage and an automatically allocated loopback
port for the live smoke. Load the operator-supplied key only in that process,
discover aliases, and send one mixed-primitive request. Record sanitized model and
answer-type/usage evidence. Existing coding-agent live loops require compatible
generation providers; Jev cannot run them, and any missing prerequisites must be
reported concretely. Archive only after all tasks and verification pass.

## Approval state

The user approved all change artifacts with "proceed" after strict validation.
