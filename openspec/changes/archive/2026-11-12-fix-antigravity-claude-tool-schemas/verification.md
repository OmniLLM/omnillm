## Approval and deterministic regression

The proposal, provider/gateway deltas, design, tasks, isolated credential use, bounded native-client verification, deployment, and archival were approved after strict validation passed (15 items).

Before implementation, the new regressions failed for both Claude models across Messages, Chat Completions, and Responses input in streaming and collected mode. They also showed the three gateway routes incorrectly executing the collected fallback despite an explicit wrapped-error restriction. After implementation, the Antigravity/routes/server race suites passed.

The provider regression preserves nested unions, discriminator constants, references, required properties, boolean schemas, a property literally named `const`, descriptions, model identity, and the complete canonical input. Existing nil-schema coverage and the explicit Gemini representation regression passed. The error-policy tests cover seven representative 4xx statuses and three 5xx statuses, with wrapped policy behavior verified through all three public streaming routes.

## Automated checks

- `bun run spec:check`: strict validation and mandatory gate passed.
- `bun run lint:all` and `bun run typecheck`: passed.
- `bun run test`: 380 passed, 23 skipped, 0 failed.
- `bun run build`: passed; existing bundle-size advisory remains.
- `go vet ./...`, `go build ./...`, and `go test -race ./...`: passed.
- `git diff --check`: passed.
- The complete Go suite includes the deterministic five-call/five-result terminal fixtures for Claude Code, Codex CLI, Droid, and GitHub Copilot CLI custom-provider mode.

## Native verification history

Client versions: Claude Code 2.1.322, Codex CLI 0.167.1, GitHub Copilot CLI 1.0.109, and Droid 1.2.0.

The first isolated Claude Code trial received upstream HTTP 429 `RESOURCE_EXHAUSTED` on ten requests and was interrupted by the outer diagnostic timeout. It did not establish native-client success. The diagnostic Claude process and its own gateway were stopped and temporary credential state removed. Subsequent 1024-token plain requests on both working accounts returned HTTP 200, motivating a bounded 2048-token native verification run with request metadata retained. No account-capacity failure is counted as a successful schema or tool-loop check.

The final isolated and installed-service verification results are recorded below.

## Continuation extension verification

The user approved extending the plan to fix the observed continuation defect, then approved implementation after the revised artifacts passed strict validation. Before this extension, the new five-exchange converter tests failed on call/result IDs and missing names for all four client shapes. The stream tests also rejected replacement of a supplied native tool ID. Both test groups passed after the ID/name and stop-semantics changes.

The controlled two-turn check now returns HTTP 200 with `tool_use` for its first call (`d9ff83d35002be00`), then HTTP 200 with `end_turn` and exact `CONTINUATION_OK` for the result continuation (`12a79fdb03dd5da4`). This confirms the formerly rejected replay, but is not a substitute for the required five-call native client tests.

The updated shared/Antigravity/routes race suites passed. One combined run encountered an existing registration/persistence timing failure (`provider is busy`) in `TestCatalogRefreshRestoresAllClientToolHistories`; that unchanged test passed in isolation, and the subsequent complete `go test -race ./...` passed. Final Go vet/build and OpenSpec checks passed. The previously passing Bun lint/typecheck/test/build inputs were unchanged by the continuation extension.

## Other native clients: verified

After the continuation fix and the Codex test-shell PATH correction, an isolated gateway on `127.0.0.1:43491` completed the following real client runs against `antigravity/claude-sonnet-4-6`:

| Client | Native tool | Calls/results | Ordered and sequential | Exact terminal marker | Exit | Duration |
| --- | --- | --- | --- | --- | --- | --- |
| Codex CLI | command execution (`cat`, read-only sandbox) | 5 / 5 | Passed | Passed | 0 | 16.52s |
| GitHub Copilot CLI custom provider | `view` | 5 / 5 | Passed | Passed | 0 | 13.02s |
| Droid | `Read` | 5 / 5 | Passed | Passed | 0 | 29.26s |

Each run consumed a generated five-file chain, with each result revealing the next filename, and ended without a sixth call. The gateway recorded no provider failures in this run. Its private temporary credential database was removed after shutdown. The earlier failed attempts remain recorded as failures; they are not classified as skipped.

## Claude Code verification-mode distinction

The initial print-mode harness continued receiving upstream 429 even with two tools, a 2048-token limit, and the same compatibility settings as the user. A controlled comparison isolated the system text: the exact full 24-tool catalog (104,468 serialized characters) succeeded in both streaming and collected mode with an ordinary system prompt. Retaining the print-mode system text failed; removing only that text or replacing it with the synthetic test system text succeeded. Joining the original blocks into one string still failed, excluding block shape as the cause.

The generated SDK identity prefix `You are a Claude agent, built on Anthropic's Claude Agent SDK.` reproduced HTTP 429 by itself. The generated interactive identity prefix `You are Claude Code, Anthropic's official CLI for Claude.` returned HTTP 200 on the same account, model, tools, and diagnostic prompt. This is observed upstream behavior, not a claim about its internal enforcement mechanism or quota policy.

Verification therefore uses actual interactive Claude Code in a private pseudo-terminal, forwards its native system prompt unchanged, and checks tool/result history plus visible terminal output and clean client exit. No identity text is stripped, replaced, or injected by OmniLLM. Print-mode failures remain a separately recorded upstream limitation.

## Interactive Claude Code: verified on both models

Real interactive Claude Code 2.1.322 ran in private pseudo-terminals without `--print` and without a system-prompt override. Its original interactive identity and default system prompt were forwarded unchanged. Each request advertised 28 tools, including SendMessage. The only approved executed tool was Read, and each of five generated files revealed the next filename. The final visible answer contained all five unpredictable markers in order. The client then exited normally using its local exit command.

| Phase | Model | Gateway | Calls/results | HTTP requests | Terminal / exit | Duration |
| --- | --- | --- | --- | --- | --- | --- |
| Isolated | `claude-sonnet-4-6` | `127.0.0.1:42481` | 5 / 5 | 6 × 200 | Exact visible marker, `end_turn`, exit 0 | 19.86s |
| Isolated | `claude-opus-4-6-thinking` | `127.0.0.1:44677` | 5 / 5 | 6 × 200 | Exact visible marker, `end_turn`, exit 0 | 25.35s |
| Installed service | `claude-sonnet-4-6` | `127.0.0.1:5000` | 5 / 5 | 6 × 200 | Exact visible marker, `end_turn`, exit 0 | 20.21s |
| Installed service | `claude-opus-4-6-thinking` | `127.0.0.1:5000` | 5 / 5 | 6 × 200 | Exact visible marker, `end_turn`, exit 0 | 23.57s |

Every row verified five unique calls, ordered matching results, sequential observation, no sixth tool call, SendMessage on the actual request wire, and no timeout. The smoke output budget was 2048 tokens per response. The user's existing third-party-provider compatibility flags were used; deferred tool search was disabled for deterministic schema coverage. These are bounded native-client checks, not a claim that every optional MCP/plugin configuration or upstream quota condition is supported.

The first interactive harness attempt stopped at its private API-key confirmation prompt before making any request. The authorized confirmation handling was corrected, and all four final interactive runs passed. This setup failure is not counted as an upstream or model failure.

## Deployment and service corroboration

`make install` completed and `omnillm.service` was restarted. At 2026-11-12 07:41:52 CST the service was active and `/health` returned healthy. Installed binary SHA-256: `9098e0d6b91f0c66e771ea73bb0e34bd5a22719f94ccc681e328311ab36682c3`. The previous binary is retained at `/tmp/opencode/antigravity-schema-fix-pk1epc82/omnillm.before-schema-fix` for rollback.

The service journal independently records terminal streaming HTTP 200 responses from provider `antigravity`, with the requested model preserved:

- Sonnet: request `f08132825ff90e2d`, 2026-11-12 07:42:23 CST, `end_turn`.
- Opus: request `68f40642cb1e447a`, 2026-11-12 07:42:46 CST, `end_turn`.

All final seven native verification rows (four Claude Code rows plus Codex, Copilot, and Droid) passed. Every isolated gateway was stopped, copied credential databases were removed, and private diagnostic client configurations were cleaned up. Sanitized durable counters and terminal evidence are stored in `client-verification.json`. All tasks are complete. The validated OpenSpec archive command synchronized the three provider/gateway requirements and archived this change on 2026-11-12.
