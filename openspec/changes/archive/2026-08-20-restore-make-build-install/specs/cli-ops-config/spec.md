## MODIFIED Requirements

### Requirement: Minimal Make orchestration

The repository Makefile MUST be limited to building, installing, and
uninstalling the canonical `omnillm` binary and orchestrating desktop sidecar
workflows, and MUST NOT wrap OmniLLM lifecycle, Bun development, validation,
release, or container commands.

#### Scenario: Server lifecycle

- **WHEN** an operator starts, stops, restarts, inspects, or follows logs for the packaged gateway
- **THEN** current documentation directs the operator to `omnillm` rather than a Make target

#### Scenario: Canonical binary build

- **WHEN** a contributor runs `make build`
- **THEN** the root Go package is compiled as `omnillm` under `.build/bin` for the host platform

#### Scenario: Canonical binary installation

- **WHEN** a contributor runs `make install`
- **THEN** the root Go package is installed through `go install .` using the configured Go installation path

#### Scenario: Canonical binary uninstallation

- **WHEN** a contributor runs `make uninstall`
- **THEN** only the installed `omnillm` executable and any stale legacy `omniproxy` executable are removed from Go's effective binary installation directory and absent executables are treated as success

#### Scenario: Retained Make surface

- **WHEN** a contributor inspects public Make targets
- **THEN** only `build`, `install`, `uninstall`, `build-desktop-sidecar`, `build-desktop`, and `desktop-dev` are available

#### Scenario: Desktop workflows

- **WHEN** a contributor invokes a retained desktop Make target
- **THEN** `build-desktop-sidecar`, `build-desktop`, and `desktop-dev` continue to provide cross-platform desktop orchestration

#### Scenario: Direct development tools

- **WHEN** a contributor tests, lints, releases, or runs a container outside desktop packaging
- **THEN** current documentation uses the direct Bun, release-script, or Docker command
