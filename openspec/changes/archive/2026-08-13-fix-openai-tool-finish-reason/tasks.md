## 1. Regression Coverage

- [x] 1.1 Add a serializer test where valid canonical tool calls coexist with a generic terminal stop reason and assert OpenAI `finish_reason: tool_calls`.
- [x] 1.2 Preserve coverage showing ordinary responses without tool calls serialize with `finish_reason: stop`.

## 2. Tool Finish Normalization

- [x] 2.1 Normalize OpenAI-compatible non-streaming finish reasons from canonical tool-call content while preserving existing response fields.
- [x] 2.2 Require the named tool in the real-server compatibility scenario so every supported model is deterministically exercised through tool replay.

## 3. Verification

- [x] 3.1 Run focused serialization and server compatibility tests.
- [x] 3.2 Run the standard Go, Bun, and OpenSpec checks.
- [x] 3.3 Rerun the real-server live tool-call matrix, record every pass, failure, and credential skip, then archive the change if verification passes.
