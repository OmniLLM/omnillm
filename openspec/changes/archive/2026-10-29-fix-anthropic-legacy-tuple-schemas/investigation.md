## Observed production behavior

On 2026-10-29 (all times CST, UTC+8), Claude Code requests using `claude-sonnet-5.5` and 202 tools completed at 21:26:49. A zero-tool request succeeded at 21:28:05. Requests with expanded tool sets began failing at 21:28:06 with upstream errors such as `tools.162.custom.input_schema: JSON schema is invalid. It must match JSON Schema draft 2020-12`.

The inspected production window contained 17 schema-related failures and three separate model-unavailable failures for `claude-opus-5-5`. The service remained running with zero automatic restarts. The available Copilot catalog uses `claude-opus-5.5`.

## Tool identification

A bounded Claude Code diagnostic in the affected `/home/jzhu/repos/personal-hub` project sent its request to a temporary loopback HTTP listener instead of a model endpoint. The listener retained tool definitions only, omitted headers and conversation content, returned a terminal diagnostic error, and stopped. No model or MCP action was requested by the diagnostic listener. MCP initialization and tool discovery used the existing client configuration.

Of the 170 tool definitions observed in that fresh session, draft 2020-12 schema checking identified an invalid schema on `mcp__plugin_ui-color-palette_ui-color-palette__create_color_harmony`. Its `baseColor` property uses three ordered integer schemas bounded from 0 to 255 under array-valued `items` and declares draft-07.

Session tool counts and indexes vary with successful MCP connections; this fresh capture does not map every historical index to a tool name.

## Live controlled comparison

Two bounded non-streaming requests used the installed gateway, the same `claude-sonnet-5.5` route, `max_tokens: 16`, the same single color-harmony tool, `tool_choice: none`, and a request to reply OK. Authentication was loaded without printing credentials.

- Original legacy tuple schema: HTTP 502, client envelope `All providers failed`.
- Equivalent tuple schema using ordered `prefixItems`: HTTP 200, `stop_reason: end_turn`, response `OK.`.

No tool action executed in either request. No repository implementation, runtime configuration, or service restart was performed for this comparison.

## Conclusion and limits

The color-harmony tool supplies a legacy tuple construct that OmniLLM does not migrate before sending it to a backend requiring draft 2020-12. Converting that construct resolves the isolated request rejection. This proves the schema defect and the narrow migration, not full coding-client recovery or the absence of other invalid tools in other sessions.
