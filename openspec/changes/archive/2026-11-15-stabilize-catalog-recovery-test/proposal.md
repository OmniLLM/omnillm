## Why

The catalog recovery compatibility test intermittently fails with `provider is busy`; fixture replacement can overlap a background provider save holding a lifecycle lease. This has blocked PR CI and can be reproduced locally under the race detector.

## What Changes

- Stabilize the catalog recovery test by synchronizing fixture replacement with outstanding provider persistence and any other relevant lifecycle work.
- Preserve coverage of tool histories for all supported client fixtures and verify repeated race-test runs.
- Make no production runtime behavior, API, or provider lease changes.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `model-compatibility-testing`: Require reliable catalog-recovery coverage across supported client tool histories.

## Impact

Changes are limited to deterministic test setup and verification, plus the OpenSpec artifacts. No runtime behavior or dependencies change.
