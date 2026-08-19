# Add a GitHub Copilot Provider Alias

## Why

The configured GitHub Copilot provider instance
`github-copilot-jian-zhu---zhujian0805` is difficult to reference in
provider-qualified model names. A short alias makes routing commands and model
references easier to read without changing the provider implementation.

## What Changes

- Set the provider instance subtitle/alias to `jzhu` in the configured OmniLLM
  database.
- Preserve the provider instance ID, credentials, activation state, model
  catalog, and all existing provider behavior.
- No source-code, schema, or API implementation changes are required because
  provider subtitle aliases are already supported by the current runtime.

## Capabilities

- **Modified:** provider routing alias resolution.
- **Modified:** provider-instance configuration persistence.

## Runtime Behavior Impact

This change affects runtime configuration: `jzhu/<model>` will resolve to the
existing GitHub Copilot provider instance. It does not add a new provider type
or alter request translation.
