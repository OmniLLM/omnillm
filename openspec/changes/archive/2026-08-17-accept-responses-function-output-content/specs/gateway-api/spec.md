## ADDED Requirements

### Requirement: Responses function-tool history input
The `/v1/responses` endpoint SHALL accept an ordinary `function_call_output` whose explicitly present `output` is either a string or an ordered list containing only `input_text`, `input_image`, or `input_file` content items valid under the maintained OpenAI Responses schema, SHALL preserve the associated `call_id`, and SHALL process the normalized history through ordinary provider dispatch. The endpoint SHALL reject missing outputs, unsupported output value kinds, malformed content members, unsupported member types, and invalid values for fields that are present with HTTP 400 and a structured `invalid_request_error` before provider dispatch.

#### Scenario: String function output continuation
- **WHEN** a client submits a Responses history containing a function call and a string output with the matching `call_id`
- **THEN** the gateway accepts the request and reaches provider dispatch with the call/result association preserved

#### Scenario: Structured function output continuation
- **WHEN** a client submits a Responses history containing a function call and an ordered valid text, image, or file output list with the matching `call_id`
- **THEN** the gateway accepts the request, preserves the complete ordered output, and reaches provider dispatch

#### Scenario: Explicit empty function output
- **WHEN** a function output is an explicitly present empty string or empty content list
- **THEN** the gateway accepts it and distinguishes it from an omitted output

#### Scenario: Missing function output
- **WHEN** a `function_call_output` omits `output`
- **THEN** the gateway returns HTTP 400 with a structured `invalid_request_error` before provider dispatch

#### Scenario: Invalid function output value
- **WHEN** a `function_call_output.output` is an object, scalar other than string, null, or a list containing a non-object member
- **THEN** the gateway returns HTTP 400 with a structured `invalid_request_error` before provider dispatch

#### Scenario: Type-only and nullable image or file content
- **WHEN** a function output list contains an `input_image` or `input_file` member with only its required type, or with optional fields explicitly set to null
- **THEN** the gateway accepts the schema-valid member, preserves it unchanged, and reaches provider dispatch

#### Scenario: Invalid function output content item
- **WHEN** a function output list contains an unsupported type, an `input_text` member without string text, or an optional image/file field with an invalid supplied type or enum value
- **THEN** the gateway returns HTTP 400 with a structured `invalid_request_error` before provider dispatch
