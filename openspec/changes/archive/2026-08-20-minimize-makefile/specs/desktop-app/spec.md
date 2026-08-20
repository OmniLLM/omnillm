## ADDED Requirements

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
