## ADDED Requirements

### Requirement: Classified inbound body-read diagnostics
When a bounded request-body read fails for Chat Completions, Responses, Messages,
Messages token counting, or System One, the gateway SHALL emit one diagnostic
for the read failure with a stable reason classification. It SHALL report
unexpected EOF and body-limit violations at WARN, explicit cancellation matching
a canceled request context at INFO, and other read failures at ERROR. It MUST
retain existing HTTP 400 and HTTP 413 mappings and endpoint error envelopes,
and MUST NOT parse, dispatch, or retry a partial request body.

#### Scenario: Truncated upload
- **WHEN** a request body reader returns an error matching `io.ErrUnexpectedEOF`
- **THEN** the gateway emits a WARN diagnostic classified as `unexpected_eof`, attempts the existing HTTP 400 structured response, and does not parse or dispatch the partial body

#### Scenario: Explicit request cancellation
- **WHEN** the body-read error matches `context.Canceled` and the request context is canceled
- **THEN** the diagnostic is INFO with reason `canceled` and the handler follows its existing HTTP 400 error path without provider execution

#### Scenario: Body limit violation
- **WHEN** the bounded reader determines that the body exceeds 16 MiB
- **THEN** the diagnostic is WARN with reason `too_large` and the existing HTTP 413 structured response is retained

#### Scenario: Unclassified failure with canceled context
- **WHEN** an unrelated body-read error occurs while the request context is canceled
- **THEN** the failure remains ERROR with reason `read_error` rather than being reclassified solely because the context is canceled

### Requirement: Metadata-only upload failure evidence
Body-read failure diagnostics SHALL include the request identifier, API shape,
HTTP method, path without query parameters, trusted client address, user agent
bounded to 512 bytes at a UTF-8 boundary, received-byte count, declared content
length, and a normalized request-context state. They MUST NOT include partial
body contents, arbitrary reader-error strings, query values, authentication
headers, or credentials. Unknown declared content length SHALL remain explicit
rather than being represented as zero.

#### Scenario: Partial bytes returned alongside failure
- **WHEN** a read returns partial data and an error in the same call
- **THEN** the diagnostic counts those bytes, the failed reader exposes no usable payload to parsing, and no partial content appears in logs

#### Scenario: Chunked upload fails
- **WHEN** a request with unknown content length fails during body reading
- **THEN** the diagnostic reports the received-byte count and `content_length=-1`

#### Scenario: Sensitive failure input
- **WHEN** the body, reader-error text, query string, or authentication headers contain a synthetic secret sentinel
- **THEN** neither the failure diagnostic nor its associated HTTP access record contains that sentinel

#### Scenario: Successful large upload
- **WHEN** a complete valid body is within the existing limit
- **THEN** normal parsing and dispatch remain unchanged and no body-read failure diagnostic is emitted

### Requirement: Query-free HTTP access paths
HTTP access records SHALL identify the URL path without query parameters while
retaining request identifier, method, response status, and latency.

#### Scenario: Query-bearing request
- **WHEN** an HTTP request completes with query parameters in its request URI
- **THEN** its HTTP access record contains the path and status but no query parameter names or values
