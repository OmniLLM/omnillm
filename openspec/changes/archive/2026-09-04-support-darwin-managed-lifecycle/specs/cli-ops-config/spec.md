## ADDED Requirements

### Requirement: Cross-platform managed lifecycle identity

Managed lifecycle operations MUST use native process identity on Linux and
macOS, MUST reject PID reuse and unrelated processes, and MUST retain a valid
identity when the running executable is replaced in place.

#### Scenario: Managed background lifecycle on macOS

- **WHEN** a macOS operator starts OmniLLM in the background and subsequently
  invokes status and stop operations
- **THEN** the CLI records native process identity without relying on Linux
  `/proc`, reports the server as reachable, gracefully terminates that same
  process, and removes its lifecycle state

#### Scenario: Managed foreground lifecycle on macOS

- **WHEN** a macOS operator invokes `omnillm start --foreground`
- **THEN** lifecycle registration succeeds without relying on Linux `/proc` and
  the server remains attached until shutdown

#### Scenario: Restart after executable replacement on macOS

- **WHEN** a macOS operator atomically replaces the `omnillm` executable while
  its managed server remains running and then invokes `omnillm restart`
- **THEN** the CLI recognizes and terminates that same live process before
  starting its replacement

#### Scenario: Reused PID is not managed on macOS

- **WHEN** macOS lifecycle state refers to a PID whose native process start ID
  or normalized executable path differs from the recorded identity
- **THEN** the CLI treats the state as stale and does not signal that process

#### Scenario: Existing Linux lifecycle behavior

- **WHEN** a Linux operator starts, stops, or restarts a managed OmniLLM server
- **THEN** the CLI preserves the existing `/proc`-backed lifecycle identity and
  safety behavior

#### Scenario: Unsupported process identity platform

- **WHEN** lifecycle registration runs on a platform without a native process
  identity implementation
- **THEN** the CLI returns an explicit unsupported-platform error and does not
  create lifecycle state
