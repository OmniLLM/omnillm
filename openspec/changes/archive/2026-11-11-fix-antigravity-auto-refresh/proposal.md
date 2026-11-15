## Why
Antigravity requests return upstream 401 after successful OAuth because the adapter reloads the stored access token indefinitely and RefreshToken is a no-op. OAuth completion does not retain expiry. Successful login therefore does not ensure continued access.

## What Changes
- Refresh expiring credentials automatically before generation; recover legacy records without expiry.
- On upstream HTTP 401 before streaming begins, refresh or reuse a concurrently renewed token and retry once.
- Persist expiry and rotated tokens safely while preserving instance metadata and preventing stale writes over re-authentication.
- Return actionable, sanitized failures when refresh is unavailable or rejected.

## Capabilities
### New Capabilities
None.
### Modified Capabilities
- `providers`: Antigravity automatic refresh and bounded authentication recovery.
- `persistence`: Durable Antigravity token expiry and conditional credential updates.

## Impact
Changes runtime authentication for streaming and non-streaming Antigravity generation, OAuth completion, and explicit refresh. No new API or database schema is required. Existing instances retain their identity and configuration. Other providers remain unchanged. Existing security and coding-client compatibility requirements continue to apply.
