## Root cause and boundaries

`internal/ingestion/from_anthropic.go` calls `normalizeSchema` for each tool. Its current recursive normalization removes `$schema` and `patternProperties` and normalizes nullable types, but does not migrate draft-07 tuple arrays. `internal/providers/copilot/payload.go` forwards the canonical tool parameters through `shared.NormalizeToolParameters`, which only substitutes a schema when parameters are nil. Copilot subsequently rejects array-valued `items` under its draft 2020-12 contract.

The observed invalid schema belongs to the color-palette MCP tool's `baseColor` RGB tuple. A live single-tool request establishes that tuple migration alone resolves this rejection. It does not establish that every historical failure index referred to the same tool.

## Decision

Normalize legacy tuple syntax at the existing Anthropic ingestion boundary, so canonical consumers receive the compatible schema consistently. Keep the change limited to legacy tuple conversion and avoid changing unrelated ingestion or provider behavior.

Convert array-valued `items` to ordered `prefixItems`. Move associated boolean or schema-valued `additionalItems` to `items`, or omit the trailing constraint when absent. Preserve ordinary array `items`, existing modern `prefixItems` schemas, and all positional and trailing-item constraints. Do not invent a tuple length restriction: the observed original RGB schema does not explicitly prohibit extra elements.

Use a schema-aware traversal for the new conversion: visit schema values under properties, patternProperties, definitions/$defs, dependentSchemas, schema combinators, and single-schema keywords. Do not interpret literal defaults/examples/enum/const values or the property-name dictionaries themselves as schema nodes. Preserve the existing normalization behavior outside the new tuple migration. Do not mutate caller-owned maps or slices. Do not overwrite an existing `prefixItems` value in an ambiguous mixed-dialect node; leave that node's tuple structure unchanged so invalid input remains visible rather than guessing its intended semantics.

## Compatibility and failure handling

Tool names, descriptions, ordering, cache markers, and call/result relationships remain intact. No fallback removes tools. Existing malformed schemas outside this exact legacy construct remain visible as failures; general schema validation and error-envelope changes are outside scope. Chat Completions and Responses ingestion are unchanged, but their existing deterministic client loops must continue passing.

## Verification

Add focused regressions for the sanitized RGB fixture, closed and open tuples, constrained trailing elements, nested schema positions, keyword-like property names, literal examples, existing modern tuples, and source immutability. Check the generated provider payload and successful continuation through the real ingestion/provider boundary with a deterministic fake upstream. Retain five-turn fixtures for Claude Code, Codex CLI, Droid, and GitHub Copilot CLI.

Run relevant Go checks, repository-required Bun/OpenSpec checks, and bounded live client checks using isolated runtime state and automatically allocated loopback ports when prerequisites exist. Report concrete skip reasons. Repeat the original/fixed single-tool comparison after implementation without invoking MCP actions. Record sanitized evidence only.

## Rollout

After human approval, implementation, and successful checks, install the verified binary and restart the existing service. Verify actual tool-bearing Claude requests and terminal continuation, not only the health endpoint. Keep the previous binary recoverable until verification succeeds. Do not alter Claude Code's persistent MCP configuration to hide the failure.
