## Why
The official agy client successfully generates through daily-cloudcode-pa.googleapis.com. OmniLLM instead sends generation to cloudcode-pa.googleapis.com, which returned 429 for the same account. A direct diagnostic using OmniLLM's existing credentials, the daily endpoint, and gemini-3.8-flash-low returned HTTP 200, text OK, and finish reason STOP. This is a provider endpoint mismatch, not evidence of exhausted account-wide quota.

## What Changes
- Use https://daily-cloudcode-pa.googleapis.com as Antigravity's default generation endpoint and first project-discovery endpoint.
- Add gemini-3.8-flash-low, gemini-3.8-flash-medium, and gemini-3.8-flash-high to the built-in catalog, using the exact identifiers verified with agy models.
- Add daily-cloudcode-pa.googleapis.com to this host's service-specific proxy bypass, preserving the existing entries and OAuth proxy behavior.
- Rebuild, restart, refresh affected model catalog views, and verify normal and streaming generation with fresh prompts.

## Capabilities
### New Capabilities
None.
### Modified Capabilities
- `providers`: Antigravity endpoint selection and Gemini 3.8 catalog entries.

## Impact
Changes runtime outbound routing and the exposed model catalog. Existing identities, credentials, project IDs, and model settings remain valid. No schema migration or public API changes. The host service change is deployment configuration outside the repository. Prior auto-refresh and client-version changes remain in the working tree.
