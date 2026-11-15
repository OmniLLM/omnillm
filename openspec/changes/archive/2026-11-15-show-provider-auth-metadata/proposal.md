## Why

Operators cannot inspect useful provider authentication metadata, such as the OAuth authorization endpoint and client ID, using `omnillm provider show`. The existing command deliberately omits all configuration to avoid exposing credentials.

## What Changes

- Show available, explicitly allowlisted non-secret provider metadata in table and JSON output, including OAuth authorization endpoint and public client ID where applicable, plus useful safe configuration details where available.
- Provide authenticated provider-scoped metadata from the server so remotely managed instances work without the CLI reading local credentials.
- Exclude client secrets, API keys, access and refresh tokens, authorization codes, PKCE material, OAuth state, and temporary authorization URLs from all metadata output.
- Preserve existing provider reference resolution and model inspection behavior; communicate missing or unsupported metadata without inventing values.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `cli-ops-config`: Extend provider detail inspection with safe authentication and provider metadata.
- `admin-api`: Expose authenticated, provider-scoped, allowlisted inspection metadata.
- `security`: Ensure the new inspection surface does not expose credential material or temporary OAuth flow data.

## Impact

The provider inspection CLI and authenticated administration API gain runtime behavior. Provider metadata may be extracted from existing provider configuration or credential records, but no credential or persistent-state migration, authentication flow, or routing behavior changes.
