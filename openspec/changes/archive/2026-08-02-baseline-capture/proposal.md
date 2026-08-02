## Why

OmniLLM's behavior is currently distributed across implementation, tests, README files, design plans, and regression reports, so contributors and coding agents lack one enforceable source of truth. Establishing an OpenSpec baseline now preserves the brownfield system as it behaves today and makes spec-first development mandatory for every future behavior or code change.

## What Changes

- Initialize a pinned OpenSpec toolchain and repository-local workflow.
- Capture current observable behavior as strict, scenario-based capability specifications.
- Add binding agent and contributor instructions requiring proposal, delta specs, design when needed, tasks, approval, implementation, verification, and archive in that order.
- Add a diff-aware gate and CI job that reject code changes without valid OpenSpec artifacts.
- Preserve historical documentation as non-normative context linked to the specifications.
- Document this baseline capture as the one-time direct-write exception for empty current-state specs; all later modifications flow through delta specs and archive.

## Capabilities

### New Capabilities
- `gateway-api`: Public OpenAI-, Anthropic-, and Responses-compatible API, utility endpoints, streaming, and errors.
- `cif-translation`: Canonical intermediate request, response, and streaming-event translation contracts.
- `providers`: Provider registration, authentication, model discovery, execution, and compatibility behavior.
- `routing-failover`: Model resolution, dispatch attempts, failover, affinity, virtual models, and balancing.
- `admin-api`: Authenticated and intentionally public administrative API contracts.
- `admin-ui`: Administrative console pages, management workflows, and runtime configuration behavior.
- `persistence`: SQLite state, migrations, metering, logs, chats, providers, tokens, and virtual models.
- `caching`: Response-cache eligibility, lookup, persistence, invalidation, and observability.
- `cli-ops-config`: CLI binaries and commands, configuration precedence, development, build, and operations behavior.
- `security`: API authentication, SSRF controls, trust boundaries, token protection, and privileged configuration access.
- `desktop-app`: Tauri desktop packaging and embedded admin-console behavior.
- `spec-governance`: Mandatory OpenSpec lifecycle, diff enforcement, and CI governance for future changes.

### Modified Capabilities

None. This is the initial baseline and there are no existing normative capability specifications.

## Impact

- Adds `@fission-ai/openspec` as a pinned development dependency and generates repository-local OpenSpec assets.
- Adds normative content under `openspec/specs/` and lifecycle artifacts under `openspec/changes/`.
- Changes agent instructions, contributor workflow, pull-request expectations, package scripts, tests, and GitHub Actions.
- Reclassifies existing design and fix documentation as historical/non-normative without deleting its context.
- Does not intentionally change runtime API, provider, routing, persistence, CLI, UI, or desktop behavior.
