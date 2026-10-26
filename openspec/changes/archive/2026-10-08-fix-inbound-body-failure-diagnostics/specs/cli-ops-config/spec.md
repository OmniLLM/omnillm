## ADDED Requirements

### Requirement: HTTP outcome visibility in log tails
The server-formatted log stream consumed by `omnillm logs` and the console SHALL
retain HTTP method, query-free path, status, request identifier, and latency when
those fields exist in the source event. Ordinary CLI output SHALL display them
unless the operator explicitly requests hidden fields.

#### Scenario: Rejected upload in log tail
- **WHEN** an HTTP access event for a rejected Responses upload contains POST, `/v1/responses`, status 400, a request identifier, and latency
- **THEN** server stream formatting and ordinary CLI rendering preserve all five fields for correlation with the body-read diagnostic

#### Scenario: Successful request in log tail
- **WHEN** an HTTP access event contains status 200
- **THEN** the formatted output retains status 200 alongside its method and path

#### Scenario: Existing response-summary formatting
- **WHEN** a response-summary event has model and token metadata without HTTP method, path, or status
- **THEN** formatting preserves existing model aliases and redundant-field suppression without inventing HTTP fields
