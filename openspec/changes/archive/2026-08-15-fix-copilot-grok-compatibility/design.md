## Context

See `proposal.md` for motivation. GitHub Copilot model availability is account-dependent. `GetModels` reads the authenticated catalog and publishes an immutable model-to-shape cache from `supported_endpoints`; request dispatch then applies Copilot single-upstream rules before the adapter chooses Chat Completions or Responses. The live matrix already supplies temporary state, bounded execution, secret redaction, and automatic loopback-port allocation, but it has no Copilot Grok row and requires a literal model identifier.

## Goals / Non-Goals

**Goals:**

- Reproduce the failure against the exact Grok models and endpoint metadata visible to the authenticated Copilot account.
- Keep the live run isolated from the normal OmniLLM database and allocate a new port for every diagnostic and verification process.
- Make the smallest Copilot or routing correction supported by captured evidence.
- Preserve all Chat, Messages, Responses, streaming, tool-loop, retry, timeout, and cancellation contracts.

**Non-Goals:**

- Add an xAI provider or call `api.x.ai` directly.
- Treat a Grok marketing name as sufficient endpoint or capability metadata.
- Fix unrelated generic provider, structured-output, cache, or SSE issues not reproduced by this investigation.
- Make credentialed live tests a required CI job.

## Decisions

### Discover Grok through the existing Copilot provider

Use the authenticated Copilot `/models` response as the source of truth for model IDs, capabilities, limits, and supported endpoints. The local provider catalog currently exposes `grok-4.5` and `grok-4.6`, but the tests must use what the selected account actually advertises.

A direct xAI/OpenAI-compatible row was rejected because the user-reported models are served by GitHub Copilot and exercise different authentication, headers, endpoint selection, fallback, and timeout behavior.

### Provision only into isolated live state

The operator supplies an existing long-lived GitHub credential through an environment reference. The live runner provisions `github-copilot` inside its temporary HOME/database and never copies the normal SQLite database or logs credential values. Account-specific rows remain in ignored `scripts/live-model-matrix.json`; the tracked example documents the safe shape.

Importing the normal OmniLLM database was rejected because it would couple the test to ambient activation, provider priority, cached models, and user state.

### Reproduce before choosing the production fix

Run smoke and extended matrices first and record the exact model, selected upstream path, stable upstream status/code, failing scenario, and terminal evidence. Candidate correction points are limited to Copilot catalog/shape selection, pinned-route preparation, Copilot payload/stream transforms, or per-instance routing-cache invalidation. Only the component demonstrated by the trace will change.

Hard-coding all Grok models as Chat or Responses was rejected because Copilot can change `supported_endpoints` independently by account and model.

### Convert every live failure into deterministic evidence

Use fake Copilot `/models`, Chat, and Responses endpoints to reproduce the exact failure. Assert stable structure—endpoint, model, system instructions, tool identity and arguments, result association, terminal event, attempt count—not generated prose. Reuse the existing five-call semantic fixtures and provider strategy manifest.

Live-only acceptance was rejected because it would be credential-dependent, nondeterministic, and unsuitable as a regression gate.

### Verify with a distinct fresh process

Post-fix verification starts another live-matrix gateway with a new automatically allocated port and temporary state. Actual Claude Code, Codex CLI, Droid, and Copilot CLI smokes remain separate from the provider/model matrix and run only when their executable, authentication, custom-provider configuration, and model route are available.

## Risks / Trade-offs

- **[Risk] Copilot catalog contents vary by account** → Discover and report exact IDs and endpoints; keep account-specific rows ignored.
- **[Risk] A stale cache masks the real result** → Use a new process, temporary state, and new port for reproduction and verification; add cache invalidation only if the trace proves it is required.
- **[Risk] Model output is nondeterministic** → Assert protocol structure and bounded scenario outcomes rather than prose.
- **[Risk] Grok behavior changes upstream** → Keep deterministic fixtures based on the compatibility contract and re-run credentialed evidence when the catalog changes.
- **[Risk] A broad fix regresses other Copilot models** → Preserve existing manifest rows and retry/timeout/cancellation tests, and run the full race-enabled suite.

## Migration Plan

1. Validate and approve this OpenSpec change.
2. Discover Grok metadata, provision an isolated Copilot provider, and capture a failing fresh-port run.
3. Add the deterministic reproduction before changing production behavior.
4. Implement only the evidence-backed fix and make the regression pass.
5. Run focused and full gates, then verify Grok through a second fresh gateway process.
6. Archive only after every task and verification item passes.

Rollback removes the production correction and its new tests while retaining the live-row documentation; no stored data migration or API rollback is required.
