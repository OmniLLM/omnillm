# OmniModel

OmniModel is an enterprise-ready multi-provider LLM gateway that standardizes access to heterogeneous model backends behind a single operational surface.

It provides OpenAI-compatible and Anthropic-compatible APIs, centralized provider administration, live backend switching, and a redesigned web console for authentication, model inspection, and runtime visibility.

OmniModel is designed for teams that need to route AI traffic through a controlled internal endpoint instead of binding applications directly to individual providers.

## 🆕 Golang Backend Available

This repository now includes **two backend implementations**:
- **TypeScript/Node.js backend** (original, feature-complete)
- **🚀 Golang backend** (new, high-performance alternative)

Both backends implement the same API and can be used interchangeably. Choose based on your team's preferences and deployment requirements.

---

## Executive Summary

OmniModel acts as a control plane and gateway for model access.

It helps teams:

- present a stable API to internal tools and agents
- switch providers without changing client integrations
- centralize authentication and provider lifecycle management
- expose operational visibility through a web admin console
- support both local developer workflows and self-hosted deployments

Typical use cases include:

- internal AI gateways for engineering teams
- local or shared model routing for agent tooling
- provider abstraction for experimentation and failover
- controlled access to reverse-engineered or nonstandard backends

---

## Core Capabilities

### Unified API compatibility

OmniModel exposes endpoints compatible with both OpenAI-style and Anthropic-style clients, allowing existing applications to migrate with minimal integration changes.

### Multi-provider routing

A single deployment can manage multiple providers and switch the active backend without requiring a process restart.

### Centralized authentication

Provider authentication is handled through CLI and admin workflows, reducing the need for each downstream client to manage provider-specific credentials.

### Operational visibility

The admin console provides provider status, model discovery, usage visibility, and runtime metadata in one place.

### Developer and agent integration

OmniModel works well as a local gateway for tools such as Claude Code and other clients that expect OpenAI-compatible or Anthropic-compatible endpoints.

---

## Supported Providers

| Provider | Authentication | Operational Notes |
|---|---|---|
| **GitHub Copilot** | OAuth device flow or token | Requires active Copilot subscription |
| **Antigravity** | Google OAuth (browser) | Requires Google OAuth client credentials |
| **Alibaba DashScope** | API key or OAuth | Supports global and China regions |

Provider support is not uniform. Authentication model, quota behavior, and operational stability vary by backend.

---

## Reference Architecture

At a high level, OmniModel sits between client applications and upstream model providers.

```text
Clients / Agents / Internal Apps
            |
            v
        OmniModel
   - API compatibility layer
   - provider auth management
   - active provider selection
   - admin control surface
            |
            v
  GitHub Copilot / Antigravity / Alibaba
```

This architecture allows client applications to target a single internal endpoint while provider-specific concerns remain isolated inside OmniModel.

---

## Deployment Modes

### 1. Local developer workstation

Recommended for individual experimentation, agent workflows, and local tool integration.

### 2. Shared internal service

Recommended for teams that want a common gateway endpoint, centralized provider administration, and a consistent integration surface across internal applications.

### 3. Containerized deployment

Recommended when standardizing runtime packaging, mounting persistent state, and integrating with existing infrastructure automation.

---

## Quick Start

### Prerequisites

- [Bun](https://bun.sh) >= 1.2 (for TypeScript backend)
- [Go](https://golang.org) >= 1.22 (for Golang backend)

### Run from source (TypeScript backend - original)

```sh
git clone https://github.com/zhujian0805/OmniModel
cd OmniModel
bun install
bun run dev
```

Default endpoints:

- API: `http://localhost:4141`
- Admin UI: `http://localhost:4141/admin/`

### 🚀 Run from source (Golang backend - new)

**Simple commands:**
```sh
# Start both backend and frontend
./omni start

# Check status 
./omni status

# Stop all services
./omni stop

# View logs
./omni logs

# Restart services
./omni restart
```

**Advanced usage:**
```sh
# Custom ports
./omni start --server-port 8000 --frontend-port 3000

# Use TypeScript backend instead
./omni start --backend node

# Verbose logging
./omni start --verbose
```

Default endpoints:

- API: `http://localhost:5002`
- Admin UI: `http://localhost:5080/admin/`

**Benefits of the Golang backend:**
- Lower memory footprint
- Better performance for high-throughput scenarios
- Single binary deployment
- Faster startup time

### Run with bunx

```sh
bunx omnimodel@latest start
```

### Run with Docker

```sh
docker build -t omnimodel .
docker run -p 4141:4141 -v $(pwd)/proxy-data:/root/.local/share/omnimodel omnimodel
```

Example with direct GitHub token injection:

```sh
docker run -p 4141:4141 -e GH_TOKEN=your_token omnimodel
```

---

## Administration Experience

The redesigned admin console is available at `http://localhost:4141/admin/`.

### Provider operations

Administrators can:

- view registered providers and authentication state
- identify the currently active provider
- switch providers without restarting the service
- trigger provider authentication flows
- inspect available models per provider
- review usage and quota information where supported

### Runtime visibility

The console also exposes:

- server metadata
- active provider details
- model inventory context
- rate-limit configuration visibility

This makes the UI suitable as a lightweight operational console for day-to-day administration.

---

## CLI Operations

### Commands

| Command | Purpose |
|---|---|
| `start` | Start the OmniModel gateway |
| `auth` | Authenticate providers without starting the server |
| `check-usage` | Print GitHub Copilot usage/quota information |
| `debug` | Print runtime, version, and path diagnostics |
| `chat` | Launch an interactive provider chat shell |

### `start` options

| Option | Alias | Default | Description |
|---|---|---|---|
| `--port` | `-p` | `4141` | Listening port |
| `--provider` | | `github-copilot` | Active provider (`github-copilot`, `antigravity`, `alibaba`) |
| `--verbose` | `-v` | `false` | Enable verbose logging |
| `--account-type` | `-a` | `individual` | Copilot plan (`individual`, `business`, `enterprise`) |
| `--rate-limit` | `-r` | none | Minimum seconds between requests |
| `--wait` | `-w` | `false` | Queue/wait instead of failing on rate limit |
| `--manual` | | `false` | Require manual approval per request |
| `--github-token` | `-g` | none | Provide GitHub token directly |
| `--claude-code` | `-c` | `false` | Guided Claude Code setup |
| `--show-token` | | `false` | Print tokens during fetch/refresh |
| `--proxy-env` | | `false` | Read proxy settings from environment variables |

### Example operational commands

```sh
# Start with default provider
bun run start

# Start on a custom port with Antigravity
bun run start --provider antigravity --port 8080

# Start with Alibaba DashScope
bun run start --provider alibaba

# Enforce a 30-second minimum interval and wait instead of failing
bun run start --rate-limit 30 --wait

# Use Copilot business plan behavior
bun run start --account-type business

# Run guided Claude Code setup
bun run start --claude-code

# Authenticate providers only
bun run auth

# Inspect Copilot quota
bun run check-usage

# Print runtime diagnostics
bun run debug
```

---

## API Compatibility Surface

### OpenAI-compatible endpoints

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/v1/chat/completions` | Chat completions |
| `GET` | `/v1/models` | List available models |
| `POST` | `/v1/embeddings` | Text embeddings |

### Anthropic-compatible endpoints

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/v1/messages` | Messages API |
| `POST` | `/v1/messages/count_tokens` | Token counting |
| `POST` | `/v1/responses` | Responses API |

### Utility endpoints

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/usage` | Active provider usage and quota |
| `GET` | `/token` | Current provider token |

### Admin API

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/api/admin/providers` | List providers with auth status |
| `POST` | `/api/admin/providers/switch` | Switch active provider |
| `GET` | `/api/admin/providers/:id/models` | List models for a provider |
| `POST` | `/api/admin/providers/:id/auth` | Authenticate a provider |
| `GET` | `/api/admin/status` | Server and provider status |
| `GET` | `/api/admin/auth-status` | Current auth flow state |
| `GET` | `/api/admin/info` | Version and port information |

---

## Security and Compliance Considerations

OmniModel centralizes provider access, but it does not eliminate the need for organizational review.

Before production deployment, evaluate:

- whether each provider is contractually approved for your environment
- whether reverse-engineered providers are acceptable under your legal and compliance policies
- how provider credentials are stored, rotated, and audited
- whether `/token` and admin endpoints should be restricted behind network controls or authentication layers
- whether logs, usage data, or prompts contain regulated or sensitive information

Recommended production controls:

- run behind an internal reverse proxy or API gateway
- restrict admin access to trusted operators
- isolate persistent state with controlled filesystem permissions
- avoid exposing diagnostic/token endpoints on public networks
- review provider-specific terms before enabling shared organizational use

---

## Claude Code Integration

### Guided setup

```sh
bun run start --claude-code
```

This flow helps operators choose a primary model and a small/fast model, then prepares a ready-to-run Claude Code command.

### Manual `.claude/settings.json` example

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://localhost:4141",
    "ANTHROPIC_AUTH_TOKEN": "dummy",
    "ANTHROPIC_MODEL": "gpt-4.1",
    "ANTHROPIC_DEFAULT_SONNET_MODEL": "gpt-4.1",
    "ANTHROPIC_SMALL_FAST_MODEL": "gpt-4.1",
    "ANTHROPIC_DEFAULT_HAIKU_MODEL": "gpt-4.1",
    "DISABLE_NON_ESSENTIAL_MODEL_CALLS": "1",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"
  }
}
```

This allows Claude Code to target OmniModel as its Anthropic-compatible endpoint.

---

## Operations and Troubleshooting

### Common checks

- confirm the service is listening on the expected port
- open `/admin/` and verify provider auth state
- run `bun run debug` to inspect runtime paths and token presence
- run `bun run check-usage` when validating Copilot quota behavior
- verify the active provider has available models before routing traffic

### Common failure modes

| Symptom | Likely Cause | Operator Action |
|---|---|---|
| No authenticated providers | Auth flow not completed | Run `omnimodel auth` or authenticate in admin UI |
| 401 / Unauthorized | Expired or invalid provider credentials | Re-authenticate provider |
| No models returned | Provider auth incomplete or upstream issue | Recheck auth and provider availability |
| Rate-limit failures | Request volume exceeds configured threshold | Increase interval or use `--wait` |

---

## Development

```sh
bun install

# Start backend + frontend dev servers
bun run dev

# Custom ports
bun run dev.ts --server-port 8080 --frontend-port 3000

# Backend only
bun run dev:server

# Frontend only
bun run dev:frontend

# Production build
bun run build

# Type check
bun run typecheck
```

Frontend source lives in `frontend/` and uses Vite + React + Tailwind v4. In development mode, Vite proxies `/api/*`, `/v1/*`, and `/usage` to the backend.

---

## License

MIT
