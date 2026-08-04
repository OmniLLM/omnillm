## ADDED Requirements

### Requirement: Metadata-only tool-loop diagnostics
Tool-loop diagnostic records SHALL retain non-payload metadata needed to correlate and troubleshoot a turn without emitting tool arguments or tool results. Applicable metadata SHALL include the request identifier, API shape, tool call identifier and name, relevant message, item, or block indexes, stream state, model and provider identity when known, tool error state, and payload byte length when available.

#### Scenario: Inbound raw and canonical tool result
- **WHEN** raw and canonical inbound tool-result diagnostics are emitted for the same turn
- **THEN** each record contains its applicable correlation, tool, index, and error metadata and contains neither a tool-result field nor result content

#### Scenario: Non-streaming outbound tool call
- **WHEN** a non-streaming response completes with a tool call
- **THEN** its diagnostic record contains applicable request, model, provider, tool, block, stream, and payload-length metadata and contains neither a tool-arguments field nor argument content

#### Scenario: Streaming outbound tool call
- **WHEN** streamed tool-argument deltas complete a tool call
- **THEN** the completed diagnostic record contains applicable request, model, provider, tool, block, stream, and payload-length metadata and contains neither a tool-arguments field nor reconstructed argument content

#### Scenario: Agent execution failure
- **WHEN** a client reports a failed Agent tool execution
- **THEN** the warning retains applicable request, model, tool, index, and error metadata and contains neither a tool-result field nor result content
