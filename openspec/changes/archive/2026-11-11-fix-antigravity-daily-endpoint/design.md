## Evidence
At 2026-11-11 22:39 CST the official CLI log cli-20261111_223926.log records generation against https://daily-cloudcode-pa.googleapis.com/v1internal:streamGenerateContent. agy models lists Gemini 3.8 Flash Low, Medium, and High with the corresponding -low, -medium, and -high identifiers. A direct request using the existing OmniLLM antigravity credentials and a 512-token output budget returned OK with STOP from the daily endpoint. The smaller 32-token budget returned HTTP 200 without visible text, so verification must assert visible output and terminal completion, not HTTP status alone.

## Decisions
Define an explicitly named daily endpoint constant and make it the generation default. Keep the existing production endpoint constant accurately named for the retained discovery fallback. Put the daily endpoint first in project discovery, followed by the existing fallback endpoints; do not retry generation across endpoints automatically. Tests using explicit local base URLs must keep working. Existing project IDs remain unchanged unless an operator performs OAuth discovery again.

Add the three exact Gemini 3.8 identifiers to the existing catalog without removing older model IDs. Preserve explicit effort suffixes; do not invent an alias from bare gemini-3.8-flash to a reasoning tier. Reuse established Gemini catalog metadata conventions without asserting newly verified context or output limits.

Append the exact daily hostname to OMNILLM_NO_PROXY_APPEND in /etc/systemd/system/omnillm.service. The existing wrapper already applies that value after setproxy. Preserve all other environment settings, proxy exclusions, and the proxied OAuth token endpoint. Validate the unit, reload systemd, and restart after installation.

## Verification and rollout
Add fake-upstream tests for default endpoint selection, explicit test endpoints, discovery ordering, catalog identities, and preserved effort suffixes. Run all required checks and deterministic five-call coding-client regressions. Run configured isolated live coding clients or record concrete unavailable prerequisites. Then install, restart, check the actual process bypass environment, refresh affected model-list views, and test each instance with fresh unique prompts and sufficient output budget. Verify both streaming and collected output on the known-working account. Record account-verification or other upstream failures independently.

## Risks and rollback
The daily endpoint may have account-specific eligibility; retain explicit upstream errors and do not disguise failed generations as successful tests. Existing cached unsupported-version notices may replay for identical prompts, so use fresh prompts and identify cached responses. Do not clear unrelated caches. Rollback changes the default back and removes only the added service bypass entry; no credential migration is required.
