## ADDED Requirements

### Requirement: Canonical command binary

Repository build, installation, and documentation workflows MUST produce and
advertise `omnillm` as the sole supported Go command binary and MUST NOT build,
install, or advertise an `omniproxy` command binary.

#### Scenario: Build and install commands

- **WHEN** an operator runs a supported repository Go build or installation workflow
- **THEN** the workflow produces or installs `omnillm` without producing or installing `omniproxy`

#### Scenario: Current command documentation

- **WHEN** an operator reads current repository usage or architecture documentation
- **THEN** command examples and entrypoint descriptions identify `omnillm` and do not offer `omniproxy`

#### Scenario: Go package discovery

- **WHEN** Go tooling enumerates buildable packages in the repository
- **THEN** no `cmd/omniproxy` command package is present

### Requirement: Canonical Go toolchain

Current repository module, container, continuous-integration, and
documentation configuration MUST consistently require Go 1.27 or newer.

#### Scenario: Repository Go build

- **WHEN** a contributor, CI job, or container build resolves the repository Go toolchain
- **THEN** it uses the Go 1.27 module requirement rather than an older Go release

#### Scenario: Current development documentation

- **WHEN** a contributor reads the current setup or architecture documentation
- **THEN** Go 1.27 or newer is identified as the supported toolchain
