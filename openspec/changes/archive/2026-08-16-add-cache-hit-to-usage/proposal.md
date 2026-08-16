## Why

Provider prompt-cache counters are now persisted and returned in model API responses, but the Metering API types, CLI usage output, and browser Usage page still show only aggregate input/output totals. Operators cannot tell whether a request hit, missed, or lacked provider cache reporting, nor see the token volume read from or written to cache.

## What Changes

- Derive a provider prompt-cache status with three states: `hit` when reported cache-read tokens are greater than zero, `miss` when the provider explicitly reports zero, and `unknown` when the provider supplies no cache-read counter.
- Expose nullable cache read/write, uncached-input, and TTL-specific write counters plus derived status in raw metering records.
- Add cache-aware aggregates to usage statistics and model/provider/client breakdowns without treating unknown values as misses.
- Show cache status and token detail on the browser Metering page, including clear visual distinction among hit, miss, and unknown.
- Add equivalent cache columns and summaries to CLI usage table output while preserving raw JSON compatibility.
- Keep provider prompt-cache status separate from OmniLLM exact-response-cache hits; the latter remains identified by `X-OmniLLM-Cache: hit` unless explicitly added under a separate field in a future change.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `persistence`: Metering records and aggregates expose nullable cache details and derived prompt-cache status.
- `admin-api`: Raw and aggregate metering endpoints return prompt-cache visibility.
- `admin-ui`: The Metering workspace displays prompt-cache status and token counters.
- `cli-ops-config`: CLI usage output displays prompt-cache status and counters.

## Impact

This changes metering database query projections and aggregate result types, authenticated metering API response schemas, frontend API types and Metering page presentation, CLI usage table formatting, translations, and deterministic backend/frontend/CLI tests. It does not change provider request behavior, token accounting totals, the response-cache implementation, or database schema.
