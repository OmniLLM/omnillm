## Evidence
Google's download page https://antigravity.google/download lists the standalone Antigravity IDE as 2.6.0; the separate App and CLI have different version numbers. Checked on 2026-11-11.
A direct diagnostic request with 1.19.6 reproduced the HTTP 200 unsupported-version notice. The otherwise equivalent 2.6.0 request passed that version gate and received HTTP 429. A second account also returned 429 RESOURCE_EXHAUSTED, with a quota-related message. This proves progress past the version notice, not successful model generation.

## Decision
Define one internal constant for antigravity/2.6.0 and use it in both the HTTP User-Agent and JSON userAgent. Keep the change limited to generation metadata; do not replace the separate OAuth/discovery Google client identity. No dynamic network version discovery or new operator configuration is introduced.

## Compatibility, rollout, and failure handling
Both collected and streaming generation share Stream, so one change covers both. Preserve tools, history, model, project, and the existing authentication retry rules. Run deterministic request-contract and client-shape coverage, rebuild, restart, then perform bounded per-instance probes. Report quota errors, account verification, and upgrade notices as unsuccessful live generation. Do not retry quota failures in a loop or bypass account verification. Rollback is reverting this version change and rebuilding; credentials remain unaffected.

## Risks
Google may retire this version later; keeping the identity in one place makes future maintenance explicit. Official IDE release metadata alone does not guarantee compatibility with this internal endpoint; the live comparison is supporting evidence and full generation remains blocked by account quota. No claim of end-to-end success is justified until generation returns the requested answer.
