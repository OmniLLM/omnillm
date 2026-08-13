## ADDED Requirements

### Requirement: Responses custom-tool history input
The `/v1/responses` endpoint SHALL accept valid, explicitly typed `custom_tool_call` and `custom_tool_call_output` history items and SHALL process them through ordinary provider dispatch after canonical normalization.

#### Scenario: Droid custom-tool continuation
- **WHEN** Droid submits a Responses history containing a valid custom tool call and its associated output
- **THEN** the gateway accepts the request and reaches provider dispatch instead of returning an unknown-item HTTP 400

#### Scenario: Missing custom call identifier
- **WHEN** a custom tool call or output omits `call_id`
- **THEN** the gateway returns HTTP 400 with a structured invalid-request error

#### Scenario: Missing custom call name
- **WHEN** a custom tool call omits its name
- **THEN** the gateway returns HTTP 400 with a structured invalid-request error

#### Scenario: Missing versus empty custom input
- **WHEN** a custom tool call omits `input`
- **THEN** the gateway returns HTTP 400, while an explicitly present empty string is accepted

#### Scenario: Missing versus empty custom output
- **WHEN** a custom tool output omits `output`
- **THEN** the gateway returns HTTP 400, while an explicitly present empty string or supported empty content list is accepted

#### Scenario: Invalid custom output shape
- **WHEN** a custom tool output is neither a string nor a list of supported output content items
- **THEN** the gateway returns HTTP 400 with a structured invalid-request error
