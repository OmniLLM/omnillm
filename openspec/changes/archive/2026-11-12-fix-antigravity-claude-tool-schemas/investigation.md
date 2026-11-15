## Production incident

At 2026-11-11 23:33:07–23:33:45 CST, six gateway requests for `antigravity/claude-sonnet-4-6` failed with upstream HTTP 400 `INVALID_ARGUMENT`, identifying `tools.21.custom.input_schema` as incompatible with JSON Schema draft 2020-12. Request `28492c3113e001d7` is a representative example. Each request was retried through the non-streaming adapter path and then returned HTTP 502. Claude Code logged six matching failed attempts by model and time.

## Schema-only capture

A bounded, non-persistent Claude Code run sent requests to a temporary loopback listener. Hooks were disabled, the listener returned terminal HTTP 400, and no upstream model execution or model-requested tool execution occurred. Only tool names and schemas were retained in a private temporary directory. The capture contained 41 tool definitions. Python's existing system `jsonschema.Draft202012Validator` accepted all original, ingestion-normalized, and Gemini-sanitized schemas.

The current capture identifies SendMessage at index 18. Its `message` property accepts a string or an inner `anyOf` of structured messages with discriminator constants. A full-set request through the installed gateway returned HTTP 502 (`0e557e288f46a2bf`). A direct request with the current `parameters` envelope returned HTTP 400 identifying tool 18. A single-tool request then reproduced the same error at index 0, isolating the structural failure without relying on historical tool indexes.

## Controlled comparison

At 2026-11-11 23:42 CST, two sequential Opus requests used the same working account, endpoint, model, input tool structure, and a request for a fixed short answer:

| Function schema transport | Upstream result |
| --- | --- |
| Current sanitized `parameters` | HTTP 400, `tools.0.custom.input_schema` rejected |
| Full `parametersJsonSchema` | HTTP 200, exact `SCHEMA_OK`, terminal `STOP` |

An initial full-set Sonnet comparison returned HTTP 429 on the proposed transport, and an attempt using a different account's stale access-token snapshot returned HTTP 401. Neither was counted as generation success or proof of failure of the schema transport. At 23:44 CST, a sequential follow-up on the working account with all 41 captured tools and `parametersJsonSchema` returned HTTP 200, exact `SCHEMA_OK`, and `STOP` on Sonnet. Subsequent verification must use the gateway's credential lifecycle and complete actual native-client loops.

Google's function-declaration reference describes `parameters` as a restricted OpenAPI schema and `parametersJsonSchema` as the mutually exclusive JSON Schema field: https://ai.google.dev/api/caching#FunctionDeclaration. The controlled Cloud Code result, rather than public documentation alone, establishes the relevant live compatibility evidence.

## Native continuation defect

After the schema-only implementation, isolated Codex, Copilot, and Droid runs reached a first native tool call but did not complete their loops. The Codex harness also omitted PATH and its first `cat` command failed locally; that harness defect is tracked separately.

A minimal Messages two-turn comparison then isolated the gateway defect: the initial request returned HTTP 200 and one tool call (`8d3d6cff880c55ec`), while a correctly associated result continuation returned HTTP 502 (`17042261ae5f8211`) with upstream HTTP 400 `messages.1.content.1.tool_use.id: Field required`. Code inspection confirms that the converter used by Antigravity omits the canonical tool-call and result IDs. This is separate from the initial schema rejection and requires ID-preserving history serialization. The initial response also reported `end_turn` despite containing a tool call, motivating explicit provider stop-semantics regression coverage.
