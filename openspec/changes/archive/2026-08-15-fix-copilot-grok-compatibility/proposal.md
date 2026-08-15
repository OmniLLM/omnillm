## Why

GitHub Copilot advertises Grok models to OmniLLM, but those models lack provider-specific compatibility evidence and currently fail real requests. We need an isolated live reproduction plus deterministic regression coverage so Grok uses the Copilot endpoint and request shape advertised for the authenticated account without weakening existing Copilot models.

## What Changes

- Add credential-gated live matrix coverage for Copilot-advertised Grok models using a temporary OmniLLM home and a newly allocated loopback port.
- Discover each Grok model's identifier, capabilities, and supported upstream endpoints from the authenticated Copilot model catalog rather than treating Grok as a direct xAI provider.
- Correct only the Copilot model selection, request translation, streaming, or runtime cache behavior demonstrated by the live failure.
- Add deterministic fake-upstream coverage for the reproduced defect, all affected gateway shapes, and sustained five-call coding-agent tool loops.
- Preserve existing retry, timeout, cancellation, model-routing, and non-Grok Copilot behavior.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `providers`: Require GitHub Copilot models to execute through the upstream endpoint and request contract advertised by the authenticated Copilot catalog.
- `model-compatibility-testing`: Require isolated, credential-gated compatibility evidence for Copilot-advertised Grok models and deterministic regression coverage for any demonstrated defect.

## Impact

Likely implementation areas are the GitHub Copilot model catalog, endpoint-shape selection, request/stream adapters, provider dispatch, and the isolated live model matrix. The public Chat Completions, Messages, and Responses APIs remain compatible; no direct xAI provider, new dependency, credential format, or database migration is introduced.
