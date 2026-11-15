## Context

`provider show` currently reads the provider list and intentionally filters out configuration. The provider list does not expose all requested OAuth metadata: Antigravity stores its client ID alongside sensitive credential fields, while OpenAI and GitHub Copilot use public client IDs and stable provider-specific OAuth endpoints. OAuth login URLs can contain per-flow state and PKCE-related parameters and must not be used as inspection metadata.

## Goals / Non-Goals

**Goals:** Show useful, available provider metadata in both CLI formats for local and remote servers without exposing secrets or performing authentication.

**Non-Goals:** Print a usable one-time login URL, reveal secrets, modify provider credentials or state, change login or routing semantics, or automatically deploy the change.

## Decisions

- Add an authenticated read-only provider-reference-scoped metadata endpoint rather than having the CLI read its local database. Reuse server reference resolution and lifecycle protection, and do not initiate OAuth or contact external providers. This works when the CLI and server run on different machines.
- Return a typed explicit allowlist: provider identity; stable provider-owned OAuth authorization or device authorization endpoint and token endpoint when applicable; public OAuth client ID when available; and applicable safe metadata such as endpoint origin, region, plan, API format, and API version. Only return factual values derived from the actual provider type and configured data. Do not equate supported OAuth with the method a particular instance used.
- For Antigravity extract only `client_id` from the existing credential record without returning or logging the record; for public fixed client IDs and endpoints reuse the relevant provider constants or expose safe accessors. API-key-only or unknown providers omit OAuth fields. Do not copy raw config maps or credential JSON into responses.
- Treat configured URLs as untrusted: for operator-provided API endpoints expose at most parsed scheme, host, and port, stripping userinfo, path, query, and fragment. OAuth endpoint URLs are stable provider-owned values, not session-specific URLs. Reject malformed or non-HTTP(S) URLs and avoid logging them.
- Render the server's allowlisted metadata in both table and JSON. For older servers lacking the new endpoint, retain existing show output with a clear warning; do not silently turn authentication or server failures into successful metadata. Preserve all existing model-show and provider-reference behavior.
- Use synthetic secret sentinels in server and CLI tests, covering unknown fields, client secrets, tokens, URL components, errors, and logs. Verify behavior for OAuth providers, API-key providers, absent metadata, ambiguous references, and older servers.

## Risks / Trade-offs

- [Client ID comes from a credential record] → Read only the explicitly selected public field; never return, print, or log the source record. Keep the operation authenticated.
- [Configured endpoint paths can contain secrets] → Show only the endpoint origin rather than attempting to guess which path or query pieces are safe.
- [Client and server versions differ] → Support existing show fields when the server does not implement metadata; provide an explicit warning. Other failures remain errors.
- [A provider supports more than one auth method] → Label endpoint/client-ID fields as available OAuth metadata, not proof of the instance's actual login method.

## Migration Plan

Deploy the server before or with the CLI to expose the new metadata. Older CLI clients remain compatible with the additive endpoint; newer CLI clients retain existing details against older servers. Roll back by restoring the previous binary; existing persisted provider data is unchanged.
