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

- [Bun](https://bun.sh) >= 1.2
- [Go](https://golang.org) >= 1.22 (for Golang backend)

### Development mode (foreground, Ctrl+C to stop)

```sh
bun run dev
```

Starts both backend and frontend. The frontend is available at `http://localhost:5080` and proxies API calls to the Go backend at `http://localhost:5002`.

Options:

```sh
# Use TypeScript backend
bun run dev --backend node

# Custom ports
bun run dev --server-port 8000 --frontend-port 3000
```

### Background mode with service management

For persistent services with start/stop/status/log management:

```sh
# Start both backend and frontend (runs in background)
bun run omni start

# Check service status
bun run omni status

# Stop all services
bun run omni stop

# View recent logs
bun run omni logs

# Restart services
bun run omni restart

# Rebuild and restart (rebuilds Go binary + frontend)
bun run omni restart --rebuild

# Rebuild entire stack with custom ports and verbose logging
bun run omni restart --rebuild --server-port 5000 --frontend-port 5080 -v
```

Options:

```sh
# Custom ports
bun run omni start --server-port 8000 --frontend-port 3000

# Use TypeScript backend instead of Go
bun run omni start --backend node

# Verbose logging
bun run omni start --verbose
```

Available endpoints:

| Backend | API | Admin UI |
|---|---|---|
| Go (default) | `http://localhost:5002` | `http://localhost:5080/admin/` |
| Node (`--backend node`) | `http://localhost:4141` | `http://localhost:4141/admin/` |

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

### ⚠️ Caveat: Using Claude Code with GitHub Copilot Upstream

When you route Claude Code through OmniModel with GitHub Copilot as the upstream provider, **GitHub Copilot is charged per request** — not per token. Claude Code makes many small background calls that silently add up:

- **Sub-agent calls** — spawned for parallel tasks (search, file exploration, etc.)
- **Tool-use calls** — quick validation and formatting requests
- **Small/fast model calls** — Claude Code's internal use of "haiku" class models for lightweight tasks

**If you don't override the `_MODEL` environment variables**, Claude Code defaults to Anthropic's Haiku model for all these small tasks, which means every sub-agent invocation and tool call becomes a separate request billed against your Copilot quota.

#### Solution: Override small/fast models to a free or low-cost model

Set these environment variables to route Claude Code's lightweight calls to a cheaper or free model instead of your primary Copilot upstream.

#### Windows (PowerShell)

```powershell
$env:ANTHROPIC_DEFAULT_HAIKU_MODEL = "gpt-5-mini"
$env:ANTHROPIC_SMALL_FAST_MODEL = "gpt-5-mini"
$env:CLAUDE_CODE_SUBAGENT_MODEL = "gpt-5-mini"
```

#### Windows (CMD)

```cmd
set ANTHROPIC_DEFAULT_HAIKU_MODEL=gpt-5-mini
set ANTHROPIC_SMALL_FAST_MODEL=gpt-5-mini
set CLAUDE_CODE_SUBAGENT_MODEL=gpt-5-mini
```

#### Linux / macOS (Bash, Zsh, etc.)

```sh
export ANTHROPIC_DEFAULT_HAIKU_MODEL=gpt-5-mini
export ANTHROPIC_SMALL_FAST_MODEL=gpt-5-mini
export CLAUDE_CODE_SUBAGENT_MODEL=gpt-5-mini
```

#### Persisting across sessions

**Windows (PowerShell profile)** — add to `$PROFILE`:
```powershell
$env:ANTHROPIC_DEFAULT_HAIKU_MODEL = "qwen3.6-plus"
$env:ANTHROPIC_SMALL_FAST_MODEL = "qwen3.6-plus"
$env:CLAUDE_CODE_SUBAGENT_MODEL = "qwen3.6-plus"
```

**Linux / macOS** — add to `~/.bashrc`, `~/.zshrc`, or `~/.profile`:
```sh
export ANTHROPIC_DEFAULT_HAIKU_MODEL=qwen3.6-plus
export ANTHROPIC_SMALL_FAST_MODEL=qwen3.6-plus
export CLAUDE_CODE_SUBAGENT_MODEL=qwen3.6-plus
```

Alternatively, set them in `.env.local` or `.claude/settings.json`:

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://localhost:5000",
    "ANTHROPIC_MODEL": "claude-haiku-4.5",
    "ANTHROPIC_DEFAULT_HAIKU_MODEL": "qwen3.6-plus",
    "ANTHROPIC_SMALL_FAST_MODEL": "qwen3.6-plus",
    "CLAUDE_CODE_SUBAGENT_MODEL": "qwen3.6-plus",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"
  }
}
```

This ensures only your main conversation requests go through the per-request-billed provider, while all the small background traffic is handled by your free or flat-rate subscription — potentially saving dozens or hundreds of unnecessary Copilot requests per session.

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

# Start backend + frontend dev servers (foreground)
bun run dev

# Custom ports
bun run dev --server-port 8080 --frontend-port 3000

# Background mode with service management
bun run omni start
bun run omni status
bun run omni stop

# Backend only (TypeScript)
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
