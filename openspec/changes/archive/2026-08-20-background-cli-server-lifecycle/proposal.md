# Change: Run CLI-managed servers in the background

## Why

The new binary lifecycle commands still leave `omnillm start` and `omnillm
restart` attached to the invoking terminal. Operators expect these managed
lifecycle commands to return after successful startup so the proxy continues
running in the background and can later be controlled with `stop` or `restart`.

## What Changes

- Make `omnillm start` launch the managed server in the background by default.
- Make `omnillm restart` stop the prior managed process and launch its
  replacement in the background by default.
- Wait for a bounded startup handshake so command success means the child has
  bound its listener and registered lifecycle state; surface early child errors.
- Preserve an explicit `--foreground` mode for containers, service managers,
  debugging, and existing scripts that require attached execution.
- Detach the child from the invoking terminal and route its standard output and
  error to the existing OmniLLM log location without leaking secrets in process
  arguments or lifecycle state.

## Impact

- Affected capability: `cli-ops-config`.
- Affected code: CLI start/restart orchestration, hidden child-server entrypoint,
  lifecycle readiness, platform-specific process detachment, and tests.
- Breaking default behavior: `start` and `restart` return after readiness rather
  than remaining attached. `--foreground` retains the previous behavior.
- No provider, CIF, routing, persistence, dependency, or frontend changes.
