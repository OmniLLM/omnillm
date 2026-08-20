<div align="center">

# OmniLLM

Unified LLM gateway and control plane for OpenAI-compatible and Anthropic-compatible clients.

[中文文档](README.zh-CN.md)

</div>

OmniLLM sits between your apps, agents, coding tools, and upstream model providers. Requests are normalized into a Canonical Intermediate Format (CIF), routed to the right provider or virtual model, then serialized back into the API shape your client expects.

The practical result is simple: point your client at OmniLLM once, then switch providers, add failover, expose a web admin console, manage tool configs, and inspect usage without rewriting client code.

## Binary

This repo ships one user-facing entrypoint:

| Binary | Entry point | Role |
|---|---|---|
| `omnillm` | `main.go` | Main server and admin CLI. Starts the gateway, manages providers, models, virtual models, chat, settings, config files, and logs. |

Use `omnillm` for the gateway and all administration commands. The coding-focused `omnicode` CLI lives in its own dedicated repository.

## Why This Exists

Most teams end up rebuilding the same glue repeatedly:

- one client per provider API shape
- one-off failover and model alias logic
- ad hoc API key handling for local tools
- limited visibility into request cost, latency, and tool traffic

OmniLLM centralizes that into one gateway with:

- OpenAI-compatible endpoints: `/v1/chat/completions`, `/v1/models`, `/v1/embeddings`, `/v1/responses`
- Anthropic-compatible endpoints: `/v1/messages`, `/v1/messages/count_tokens`
- provider priority and automatic fallback
- virtual models with round-robin, random, priority, and weighted strategies
- an admin UI for providers, logs, chat sessions, metering, access tokens, settings, and tool configs

## Screenshots

![OmniLLM Admin Console](docs/assets/admin-console.png)

Additional views:

- Chat: ![Chat Interface](docs/assets/chat.png)
- Virtual Models: ![Virtual Models](docs/assets/virtual-models.png)
- ToolConfig: ![ToolConfig](docs/assets/toolconfig.png)

## Quick Start

### Option 1: Run the published package

```sh
bunx omnillm@latest start
```

Runtime defaults:

- API server: `http://127.0.0.1:5000`
- Admin UI: `http://127.0.0.1:5000/admin/`

### Option 2: Run from source

Prerequisites:

- Bun 1.2+
- Go 1.27+

Build and run the main binary:

```sh
make install
omnillm start
```

`start` waits until the server is ready, then returns while the server continues
in the background. Manage that process with the same binary:

```sh
omnillm restart
omnillm stop
```

Use `omnillm start --foreground` (or `restart --foreground`) when a container,
service manager, or debugger must remain attached to the server process.

Windows PowerShell:

```powershell
make install
omnillm.exe start
```

Use the direct Bun scripts for frontend builds, linting, tests, and development.

### API key behavior

All API and admin routes are protected. OmniLLM resolves the inbound key in this order:

1. `--api-key`
2. `OMNILLM_API_KEY`
3. `~/.config/omnillm/api-key`
4. a newly generated key, persisted to that file

Examples:

```sh
omnillm start --api-key my-secret-key
curl -H "Authorization: Bearer my-secret-key" http://127.0.0.1:5000/v1/models
curl -H "x-api-key: my-secret-key" http://127.0.0.1:5000/v1/models
```

Windows PowerShell:

```powershell
$env:OMNILLM_API_KEY = "my-secret-key"
omnillm start
Invoke-RestMethod -Uri "http://127.0.0.1:5000/v1/models" -Headers @{ Authorization = "Bearer my-secret-key" }
```

The admin UI injects the same key via a meta tag, so the browser can call the admin API without manual token entry.

## Development Workflow

OmniLLM has two distinct modes.

### Packaged/runtime mode

- one Go server
- default port `5000`
- serves both the API and `/admin/`

Start it with:

```sh
omnillm start
```

This starts a managed background process. Use `omnillm start --foreground` for
attached execution under a supervisor.

### Development mode

- Go backend on `5002` by default
- Vite frontend on `5080` by default
- admin UI is served from the Vite dev server at `/admin/`

Start both with:

```sh
bun run dev
```

Useful commands:

```sh
bun install
bun run build
bun run lint:all
bun run typecheck
bun test
bun run dev
bun run dev:frontend
omnillm status
omnillm restart
omnillm logs --follow
```

Make is limited to `make build`, `make install`, `make uninstall`,
`make build-desktop-sidecar`, `make build-desktop`, and `make desktop-dev`.
`make uninstall` removes installed `omnillm` and stale legacy `omniproxy`
executables from Go's binary installation directory.

The `bun run omni` wrapper is a development manager around the backend binary and Vite server. It is not the same thing as the production runtime path.

## Core Capabilities

### Unified API compatibility

Existing OpenAI-style and Anthropic-style clients can target OmniLLM instead of a single upstream. The routing layer and serializers handle the provider-specific differences.

### CIF translation

Incoming requests are converted into CIF before dispatch. That avoids pairwise format translation between every provider and every client shape. Adding a provider usually means implementing CIF adapters to and from that provider rather than building a matrix of conversions.

### Response cache

OmniLLM provides an optional exact-input response cache for non-streaming and streaming requests on Chat Completions, Anthropic Messages, and OpenAI Responses. It is strictly opt-in and operates at the CIF layer, so a canonical result populated through one supported API dialect or streaming mode can be re-serialized for another compatible caller.

- Eligibility is independent of sampling settings: requests may be cached whether `temperature` and `top_p` are omitted, zero, or nonzero. Sampling and other supported generation controls remain part of the semantic key, so only requests with the same generation semantics can replay an entry.
- A hit replays the previously stored canonical model result without another upstream inference. For stochastic requests, this means a prior random result may be returned again during the TTL; enable the cache only where that replay behavior is acceptable.
- Replay preserves canonical content and tool-call semantics, not the original HTTP bytes, SSE chunk boundaries, or event timing. OmniLLM serializes the stored canonical response into the current caller's API dialect and streaming mode.

Enabled state and TTL remain in the durable SQLite runtime configuration store and are read live per request. Canonical response payloads and hit counters live only in a separately supplied Redis or Valkey service:

| Setting | Default | Meaning |
|---|---|---|
| `response_cache.enabled` | `false` (opt-in) | Enable the cache |
| `response_cache.ttl_seconds` | `60` (60s) | Fallback native Redis lifetime when no valid positive TTL is configured; a positive configured TTL remains authoritative |
| `--response-cache-redis-url` / `OMNILLM_RESPONSE_CACHE_REDIS_URL` | `redis://127.0.0.1:6379/0` | Redis URL; the explicit flag overrides the environment |
| `--response-cache-redis-prefix` / `OMNILLM_RESPONSE_CACHE_REDIS_PREFIX` | `omnillm` | Namespace prefix; the explicit flag overrides the environment |

Start a local service, then enable the cache:

```sh
docker run --rm --name omnillm-redis -p 6379:6379 redis:8-alpine
omnillm start --response-cache-redis-url redis://127.0.0.1:6379/0
omnillm settings set response-cache on --ttl 60
```

`redis://user:password@host:6379/0` and `rediss://user:password@host:6380/0` URLs support authentication and TLS. OmniLLM never returns or logs the credential-bearing URL. In a container, `127.0.0.1` identifies that application container rather than a sibling Redis service; use its service hostname, for example `redis://redis:6379/0`.

Redis is optional acceleration infrastructure. If URL parsing, startup ping, authentication, reads, or writes fail, OmniLLM reports the cache backend as degraded and continues normal upstream model execution without a SQLite fallback. The health endpoints remain healthy, and bounded recovery probes restore caching when Redis returns. Upgrades intentionally discard the old disposable SQLite response rows and start cold. Existing entries retain the native TTL assigned when written; changing TTL affects new and refreshed entries and cannot resurrect an expired key.

Per-request override via the `X-OmniLLM-Cache` header: `bypass` skips the read and forces a refresh (still writes), `off` skips both read and write. Administrative statistics and clear operations are restricted to the versioned OmniLLM namespace and do not flush unrelated Redis data.

This exact-response cache is separate from **provider prompt caching**. Prompt-cache directives do not enable or alter exact-response caching, a provider prompt-cache hit still requires upstream inference unless an independent exact-response entry is served, and an exact-response hit is not reported as provider prompt-cache activity.

Redis entries use OmniLLM's versioned namespace and canonical response format. They are not byte-for-byte compatible with LiteLLM's Redis cache format and must not be treated as interchangeable.

### Provider routing and failover

Model resolution supports direct provider selection, provider-qualified model names, and automatic fallback across candidate providers when one fails. A provider can be identified by its canonical instance ID, short alias, or unique display name:

```text
provider-instance-id/model-id
alias/model-id
Provider Display Name/model-id
```

References resolve in this order: exact instance ID, unique case-insensitive alias, then unique case-insensitive display name. If an alias or display name matches multiple providers, OmniLLM rejects the reference and reports the matching instance IDs instead of choosing one.

### Virtual models

Virtual models let you expose a stable model ID to clients while mapping it to one or more upstreams. Strategies include round-robin, random, priority, and weighted routing.

### Admin surface

The admin API and UI cover:

- provider onboarding, activation, renaming, priorities, and usage
- model discovery, refresh, toggle, and version metadata
- live logs over SSE
- metering and breakdowns by provider, model, and client
- chat sessions for interactive testing
- access token management
- config-file management for external coding tools

### ToolConfig and coding-tool integration

OmniLLM can manage configuration files for tools such as Claude Code, Codex, Droid, OpenCode, and AMP. Editing external config files is intentionally guarded behind `--enable-config-edit`.

## Supported Providers

The current codebase supports these provider families in user-facing flows:

| Provider | Auth | Notes |
|---|---|---|
| GitHub Copilot | OAuth device flow or token | Default startup provider in the CLI |
| OpenAI-compatible | API key, optional depending on upstream | Ollama, vLLM, LM Studio, OpenAI, llama.cpp, and similar |
| Alibaba DashScope | API key | Supports region and plan variants |
| Azure OpenAI | API key | Configurable endpoint and deployment-based models |
| Google | API key | Generic Google provider |
| Kimi | API key | Generic Kimi provider |
| Codex | API key | OpenAI Codex provider integration |
| OpenAI (ChatGPT) | ChatGPT OAuth (PKCE) | Browser sign-in; uses a ChatGPT subscription instead of an API key |
| Antigravity | Google OAuth | Onboarded through the admin OAuth flow |

The admin UI and CLI add providers through auth-first onboarding: credentials or OAuth are validated before the provider instance is registered, so failed setup attempts do not leave unauthenticated placeholder providers behind. The same login flow can re-authenticate an existing provider without replacing its instance.

See [docs/ADDING_A_PROVIDER.md](docs/ADDING_A_PROVIDER.md) if you want to add another provider.

## API Surface

### OpenAI-compatible routes

| Method | Route |
|---|---|
| `POST` | `/v1/chat/completions` |
| `GET` | `/v1/models` |
| `GET` | `/v1/models/metadata` |
| `POST` | `/v1/embeddings` |
| `POST` | `/v1/responses` |

### Anthropic-compatible routes

| Method | Route |
|---|---|
| `POST` | `/v1/messages` |
| `POST` | `/v1/messages/count_tokens` |

### Utility routes

| Method | Route |
|---|---|
| `GET` | `/health` |
| `GET` | `/healthz` |
| `GET` | `/usage` |
| `GET` | `/token` |

### Admin routes

High-value admin groups include:

- `/api/admin/providers`
- `/api/admin/virtualmodels`
- `/api/admin/metering/*`
- `/api/admin/chat/sessions`
- `/api/admin/access-tokens`
- `/api/admin/config`
- `/api/admin/settings/log-level`
- `/api/admin/logs/stream`

## CLI Overview

The main `omnillm` binary includes:

- `start`
- `usage`
- `check-usage`
- `sync-names`
- `debug`
- `provider` (login, instances, and models)
- `virtualmodel`
- `config`
- `settings`
- `status`
- `chat`
- `logs`

### Provider management

Use one provider namespace for initial login, re-authentication, and model management:

```sh
# Create and authenticate a new provider instance.
omnillm provider login --new google --api-key "$GOOGLE_API_KEY"

# Re-authenticate the same instance using its ID, alias, or display name.
omnillm provider login my-google --api-key "$GOOGLE_API_KEY"

# Assign a short alias, then use any provider reference for model operations.
omnillm provider rename google-instance-id --alias my-google
omnillm provider model list my-google
omnillm provider model refresh "Google Production"
```

Without `--new`, `provider login <type-or-provider>` re-authenticates the referenced provider when it exists; otherwise a supported provider type starts new-provider authentication. Use `--new <type>` when you explicitly want another instance of that provider type.

Provider arguments use the same deterministic lookup as model routing: exact instance ID first, then unique case-insensitive alias, then unique case-insensitive display name. Ambiguous aliases or names fail and identify the matching instance IDs. The former root `auth`, root `model`, and `provider add` forms remain accepted as hidden deprecated compatibility commands for existing automation.

Selected `start` flags for the server binary:

| Flag | Default | Purpose |
|---|---|---|
| `--port` | `5000` | Server port |
| `--host` | `127.0.0.1` | Bind address |
| `--provider` | `github-copilot` | Default provider on startup |
| `--api-key` | generated if absent | Inbound auth key |
| `--manual` | `false` | Manual approval mode |
| `--rate-limit` | `0` | Minimum seconds between requests |
| `--wait` | `false` | Wait instead of erroring on rate limit |
| `--allow-local-endpoints` | `false` | Allow localhost and private OpenAI-compatible upstreams |
| `--enable-config-edit` | `false` | Enable editing external config files via admin API |
| `--response-cache-redis-url` | `redis://127.0.0.1:6379/0` | Redis/Valkey URL for exact-response cache storage |
| `--response-cache-redis-prefix` | `omnillm` | Versioned response-cache key namespace prefix |
| `--claude-code` | `false` | Print guided Claude Code launch configuration |

## Repository Map

| Path | Role |
|---|---|
| `main.go` | main `omnillm` CLI entrypoint |
| `internal/server/` | server bootstrap, auth, admin UI registration |
| `internal/routes/` | OpenAI, Anthropic, Responses, admin, metering, chat, config, and virtual model handlers |
| `internal/providers/` | provider implementations and adapters |
| `internal/cif/` | Canonical Intermediate Format types |
| `internal/serialization/` | response serialization back to client formats |
| `internal/database/` | SQLite-backed persistence |
| `frontend/` | React 19 + Vite admin frontend source |
| `pages/admin/` | built admin frontend served by the Go runtime |
| `tests/` | Bun-driven tests for API, frontend, and browser-facing behavior |
| `docs/` | migration notes, critical changes, and implementation guides |

## Architecture Snapshot

```text
Client or tool
  -> OmniLLM API/auth layer
  -> ingestion into CIF
  -> model resolution and virtual-model routing
  -> provider adapter execution
  -> serialization back to OpenAI, Anthropic, or Responses format
  -> metering, logging, persistence, admin visibility
```

Back-end stack:

- Go 1.27
- Gin
- Cobra
- zerolog
- modernc.org/sqlite for durable state
- Redis/Valkey (optional) for exact-response cache acceleration

Front-end stack:

- React 19
- Vite
- TypeScript
- Material UI 7
- Tailwind CSS 4

## Security Notes

Current built-in protections include:

- API-key protection for API and admin routes
- SSRF checks for OpenAI-compatible upstream endpoints
- localhost-only style CORS policy for browser use
- masked token output unless `--show-token` is enabled
- explicit opt-in for config-file editing via `--enable-config-edit`

For anything beyond local or internal use, put OmniLLM behind your own reverse proxy or gateway and restrict operator access to the admin surface.

## Build, Lint, and Test

```sh
bun run build
bun run build:go
bun run lint
bun run lint:all
bun run typecheck
bun test
bun run test:frontend
```

There are also live or environment-dependent scripts in `scripts/`, including model-matrix and provider integration checks.

### Isolated live model compatibility matrix

The live matrix is manual and credential-gated. Running any live-matrix package command without `OMNILLM_RUN_LIVE_MATRIX=1` exits successfully before reading a manifest or credentials, building the binary, creating state, allocating a port, or making a network request.

The unit tests for the live-matrix harness validate manifest parsing, multi-turn tool replay, streaming, and secret redaction without contacting an upstream model. Actual live rows send the declared OpenAI-, Anthropic-, or Responses-compatible request shapes through an isolated OmniLLM server; they require an enabled manifest and its referenced credentials. Skipped credential-gated rows are not live-call passes.

Copy `scripts/live-model-matrix.example.json` to the ignored `scripts/live-model-matrix.json` (or set `OMNILLM_LIVE_MATRIX_MANIFEST`) and declare only credential **environment-variable names** or isolated token-bundle paths. Never put secret values in the manifest. Each run builds and launches OmniLLM with a temporary `HOME`, config directory, SQLite database, and automatically allocated loopback port; temporary state is removed when the run finishes.

```sh
# Safe disabled check (no build, credential read, state access, or network)
bun run test:model-matrix:live

# Bounded availability, plain, streaming, and tool-replay checks
OMNILLM_RUN_LIVE_MATRIX=1 bun run test:model-matrix:live:smoke

# Smoke plus repeated/parallel tools, large results, long streams, and cancellation
OMNILLM_RUN_LIVE_MATRIX=1 bun run test:model-matrix:live:extended
```

Use `OMNILLM_LIVE_MATRIX_REPORT_DIR` to choose the sanitized JSON report directory. Reports classify every planned shape/scenario as `pass`, `fail`, `skipped`, or `not_applicable`; missing referenced credentials skip rows, while supplied credentials that fail provisioning or execution fail them. The compatibility command `test:model-matrix:5100` now invokes the safe smoke runner and no longer assumes port 5100 or normal user configuration.

## Spec-Driven Development

OpenSpec is the source of truth for OmniLLM behavior. Read the [current-state specifications](openspec/specs/), the mandatory [agent workflow](CLAUDE.md), and the [contributor guide](CONTRIBUTING.md) before changing code. Every code, test, dependency, or build/runtime configuration change must include a validated and approved OpenSpec change; CI enforces this with `bun run spec:check`.

Existing material under [`docs/`](docs/README.md) is supporting or historical context and does not override the current-state specifications.

## Documentation

Useful docs in this repo:

- [docs/README.md](docs/README.md)
- [docs/ADDING_A_PROVIDER.md](docs/ADDING_A_PROVIDER.md)
- [docs/CIF_MIGRATION.md](docs/CIF_MIGRATION.md)
- [docs/CONFIG_TEMPLATES.md](docs/CONFIG_TEMPLATES.md)

The `docs/` directory also contains a detailed history of critical provider, routing, streaming, and compatibility changes.
