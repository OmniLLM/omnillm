## Why

OmniLLM rejects standards-compliant OpenAI Responses histories when a `function_call_output` carries an ordered content array instead of a string. Current Codex shell results usually use strings and pass, but clients or tools that return structured text, image, or file content receive HTTP 400 and cannot continue their tool loop.

## What Changes

- Accept `function_call_output.output` as either a string or an ordered list of supported `input_text`, `input_image`, and `input_file` content items.
- Validate structured output items strictly and reject malformed, unsupported, or lossy shapes with actionable errors.
- Preserve the original structured output value in CIF while retaining compact JSON text as the portable fallback for providers that only accept textual tool results.
- Replay the original function output on native Responses provider paths and preserve existing string behavior across every provider path.
- Add deterministic ingestion, CIF, provider-payload, route, and five-turn coding-client regression coverage, then run bounded live client smokes where prerequisites are available.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-api`: Extend public `/v1/responses` request validation to accept supported structured function outputs and reject malformed shapes consistently.
- `cif-translation`: Extend ordinary Responses function-tool results with validated structured-output preservation and portable fallback semantics.
- `model-compatibility-testing`: Require deterministic and live evidence for structured `function_call_output` continuation across supported coding-agent client shapes.

## Impact

- **API behavior:** `/v1/responses` accepts an additional standards-compliant `function_call_output.output` shape; this is backward-compatible.
- **Canonical representation:** ordinary function results gain optional original-output metadata analogous to existing custom-tool output preservation.
- **Provider transforms:** native Responses adapters can replay structured output; Chat Completions, Anthropic, Gemini, and other text-only paths continue using compact JSON fallback text.
- **Tests:** Responses ingestion, CIF marshaling, Copilot/OpenAI-compatible/Azure payload construction, route compatibility, and coding-client fixtures are affected.
- **Dependencies:** no new runtime or build dependencies.
