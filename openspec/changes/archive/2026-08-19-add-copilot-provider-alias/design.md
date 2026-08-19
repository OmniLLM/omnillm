# Design: Add a GitHub Copilot Provider Alias

## Existing Mechanism

Provider instances already persist a `subtitle` value. The model-resolution
cache indexes that subtitle case-insensitively, and provider-qualified model
resolution checks exact instance IDs before subtitle aliases. The administrative
rename operation updates the persisted subtitle and refreshes the resolution
cache through the existing runtime path.

## Configuration Change

Use the existing CLI operation:

```text
omnillm provider rename github-copilot-jian-zhu---zhujian0805 --subtitle jzhu
```

This changes only provider metadata. Credentials, provider type, instance ID,
activation, priorities, model state, and model cache remain untouched.

## Compatibility and Failure Handling

The exact provider instance ID remains valid, so existing provider-qualified
models continue to work. If the provider instance is not present or the
administration server is unavailable, the command must fail without changing
the database; no direct database mutation or fallback alias is used.
