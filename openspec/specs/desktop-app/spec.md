# desktop-app Specification

## Purpose
Defines the packaged Tauri desktop application that starts and supervises an OmniLLM backend sidecar and mounts the shared administration console against it.
## Requirements
### Requirement: Desktop application shell
The application SHALL provide a resizable OmniLLM main window with enforced minimum dimensions and SHALL use the shared administration console.

#### Scenario: Application launch
- **WHEN** an operator launches the desktop application
- **THEN** a single resizable OmniLLM window opens and begins backend initialization

### Requirement: Bundled backend lifecycle
The application SHALL spawn the packaged backend sidecar on loopback with an operating-system-selected port and resolved API key and SHALL surface startup failures.

#### Scenario: Sidecar startup
- **WHEN** desktop setup completes
- **THEN** the sidecar starts with the server command, loopback host, selected port, and API key

#### Scenario: Sidecar unavailable
- **WHEN** the packaged sidecar cannot be resolved or started
- **THEN** the application displays a startup error instead of mounting the console

### Requirement: Per-install API key
The application SHALL reuse a non-empty per-install key from the platform configuration directory or generate and persist a 32-character hexadecimal key when absent.

#### Scenario: Configuration directory unavailable
- **WHEN** the platform configuration directory cannot be determined
- **THEN** the application uses a generated in-memory key for the session

### Requirement: Backend readiness
The application SHALL poll the sidecar health endpoint and expose backend connection information only after HTTP 200 is observed within 60 seconds.

#### Scenario: Readiness timeout
- **WHEN** the health endpoint does not return HTTP 200 within the timeout
- **THEN** desktop startup fails with the last observed readiness error

### Requirement: Backend bridge and restart
The application SHALL expose backend base URL, API key, and version to the WebView and SHALL support terminating, reaping, and replacing the sidecar on restart.

#### Scenario: Restart requested
- **WHEN** the WebView requests backend restart
- **THEN** the existing child is stopped, cached connection data is cleared, and a new sidecar is started on a newly selected port

### Requirement: Console bootstrap and recovery
The WebView SHALL display loading state until backend information is available, then register the desktop bridge and mount the shared console; failures SHALL offer retry.

#### Scenario: Bootstrap failure
- **WHEN** backend information cannot be obtained
- **THEN** the WebView shows the error and a retry control without mounting the console

### Requirement: Sidecar output handling

The desktop process SHALL continuously drain and log `omnillm` sidecar
standard output, standard error, process errors, and termination events.

#### Scenario: Sidecar writes output

- **WHEN** the `omnillm` child emits output on either pipe
- **THEN** the desktop process consumes it so the child cannot block on full pipes

### Requirement: Desktop build orchestration

The repository SHALL retain cross-platform Make targets for building the
canonical `omnillm` sidecar, building the packaged desktop application, and
running desktop development mode.

#### Scenario: Sidecar build

- **WHEN** a contributor runs `make build-desktop-sidecar`
- **THEN** the platform-specific script builds `omnillm-<rust-host-triple>` in the Tauri binaries directory

#### Scenario: Desktop package build

- **WHEN** a contributor runs `make build-desktop`
- **THEN** the sidecar is built before the Tauri package build runs

#### Scenario: Desktop development

- **WHEN** a contributor runs `make desktop-dev`
- **THEN** the sidecar is built before Tauri development mode starts

