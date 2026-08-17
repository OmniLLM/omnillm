## 1. Specification and Regression Fixtures

- [x] 1.1 Run strict OpenSpec validation and obtain human approval for the proposal, delta specs, design, and tasks before editing implementation code.
- [x] 1.2 Add deterministic public `/v1/responses` and ingestion fixtures for string, empty string, empty list, ordered text list, mixed text/image/file list, type-only and explicitly nullable image/file items, missing output, and malformed ordinary function outputs.
- [x] 1.3 Extend the five-exchange Responses coding-client fixture or route coverage with a structured `function_call_output` and terminal continuation while preserving existing Claude Code, Codex, Droid, and Copilot CLI shapes.

## 2. Canonical Ingestion

- [x] 2.1 Add optional original ordinary-function output metadata to CIF tool results with backward-compatible JSON marshaling and unmarshaling tests.
- [x] 2.2 Generalize the Responses tool-output normalizer and validator so ordinary outputs follow maintained OpenAI content schemas while custom outputs retain their existing supported-content semantics and context-specific errors.
- [x] 2.3 Update `function_call_output` ingestion to require an explicitly present output and call identifier, preserve strings unchanged, retain valid ordered content lists including type-only/nullable image and file members, compute compact JSON fallback text, and reject malformed or unsupported shapes atomically.

## 3. Provider Replay and Fallback

- [x] 3.1 Update GitHub Copilot native Responses payload construction to replay preserved ordinary structured output and retain existing custom/string behavior.
- [x] 3.2 Update OpenAI-compatible native Responses payload construction to replay preserved ordinary structured output and retain existing custom/string behavior.
- [x] 3.3 Update Azure native Responses payload construction to replay preserved ordinary structured output and retain existing string behavior.
- [x] 3.4 Add provider-payload and cross-dialect tests proving native Responses array fidelity and compact textual fallback for Chat Completions, Anthropic, Gemini, and other string-only paths.

## 4. Route and Client Compatibility

- [x] 4.1 Add an end-to-end fake-upstream `/v1/responses` regression for structured output continuation, call/result association, provider dispatch, and terminal completion.
- [x] 4.2 Run focused ingestion, CIF, provider, serialization, route, and five-turn compatibility tests, including malformed-input failure assertions.
- [x] 4.3 Build and launch an isolated fresh-port OmniLLM gateway and run a credentialed direct Responses probe with a structured output followed by terminal continuation.
- [x] 4.4 Run bounded five-call native Claude Code, Codex CLI, Droid, and GitHub Copilot CLI custom-provider smokes when executable, credential, model route, and safe temporary configuration are available; record sanitized call/result counts and terminal evidence or concrete skip reasons.

## 5. Complete Verification and Archive

- [x] 5.1 Run `gofmt`, `go vet ./...`, `go build ./...`, and `go test -race ./...`.
- [x] 5.2 Run `bun run lint:all`, `bun run typecheck`, `bun test`, and `bun run build`.
- [x] 5.3 Run `bun run spec:check`, review the complete diff for scope and credential safety, and confirm the working tree contains only intended changes.
- [x] 5.4 Record sanitized verification evidence, check every completed task, archive with `bunx openspec archive accept-responses-function-output-content --yes`, and rerun `bun run spec:check`.
