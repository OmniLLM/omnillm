## Why
Antigravity generation hardcodes antigravity/1.19.6 in both the HTTP User-Agent and JSON userAgent. Live requests return an unsupported-version notice as HTTP 200 instead of model output. OAuth refresh and direct network routing are already working and are separate changes.

## What Changes
- Replace the duplicated legacy version with one shared client identity using antigravity/2.6.0 for both request locations.
- Preserve existing request payload, model selection, OAuth refresh, and streaming behavior.
- Verify both outbound identity fields in deterministic tests and distinguish genuine generation from upgrade notices in live verification.

## Capabilities
### New Capabilities
None.
### Modified Capabilities
- `providers`: Consistent maintained Antigravity generation client identity.

## Impact
Runtime upstream request metadata changes for all Antigravity instances. No database, public API, or service configuration change. Account verification and quota restrictions remain upstream conditions.
