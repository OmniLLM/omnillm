## MODIFIED Requirements

### Requirement: Bundled backend lifecycle

The application SHALL package and spawn the `omnillm` backend sidecar on
loopback with an operating-system-selected port and resolved API key and SHALL
surface startup failures.

#### Scenario: Sidecar startup

- **WHEN** desktop setup completes
- **THEN** the packaged `omnillm` sidecar starts with the server command, loopback host, selected port, and API key

#### Scenario: Sidecar unavailable

- **WHEN** the packaged `omnillm` sidecar cannot be resolved or started
- **THEN** the application displays a startup error instead of mounting the console

## MODIFIED Requirements

### Requirement: Sidecar output handling

The desktop process SHALL continuously drain and log `omnillm` sidecar
standard output, standard error, process errors, and termination events.

#### Scenario: Sidecar writes output

- **WHEN** the `omnillm` child emits output on either pipe
- **THEN** the desktop process consumes it so the child cannot block on full pipes
