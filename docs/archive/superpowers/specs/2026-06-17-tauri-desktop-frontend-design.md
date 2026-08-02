# Tauri Desktop Frontend — Design

**Status:** Draft for review
**Date:** 2026-06-17
**Related:** New desktop wrapper around the existing OmniLLM admin UI.

## Goals

- Ship a desktop application that exposes the **exact same admin features** as the existing web UI, with no feature forks.
- Reuse the existing React/TypeScript admin UI in `frontend/src` rather than rebuilding it.
- Reuse the existing Go backend (`omniproxy` / `omnillm`) as the source of truth for all data and operations.
- Keep the Go backend usable as a fully standalone binary: `omnillm start`, `omniproxy start`, packaged-runtime mode (port 5000 serving `/admin/`), and dev mode (Go on 5002 + Vite on 5080) must all keep working unchanged.
- Add a desktop entrypoint that starts the bundled backend on demand and loads the shared admin UI inside a Tauri WebView.

## Non-Goals

- Building any feature inside the desktop app that does not exist in the web admin UI.
- Replacing or modifying the standalone Go server CLI behavior.
- Auto-update server, code-signing automation, or platform store distribution (deferred).
- Custom desktop-only pages beyond a minimal in-Rust “backend status / retry” fallback shown only when the sidecar fails to start.

## Architecture

```
Browser mode (unchanged):
  user/browser ─▶ Go server :5000 ─▶ /admin/ + /api/admin + /v1/*

Desktop mode (new):
  Tauri app
    ├─ launches bundled omniproxy as a managed sidecar on a free local port
    ├─ exposes desktop_backend_info() to JS (baseUrl, apiKey, version)
    └─ WebView loads the shared React admin UI
          └─ API client routes through the desktop-provided baseUrl+apiKey
```

The desktop layer is an **additional** entrypoint, not a replacement. The Go backend remains a fully independent binary.

## Repo Layout

New top-level `desktop/` package, kept separate from the Go and existing frontend code:

```
desktop/
├── package.json                # Tauri dev/build scripts only
├── tsconfig.json
├── vite.config.ts              # Vite config; alias @ → ../frontend/src
├── index.html                  # Desktop entrypoint (mounts shared App)
├── src/
│   ├── main.tsx                # imports shared AppComponent
│   ├── desktop/
│   │   ├── runtime.ts          # thin wrapper around shared runtime hook
│   │   └── desktop.css         # window-chrome tweaks only
│   └── env.d.ts
└── src-tauri/
    ├── Cargo.toml
    ├── tauri.conf.json
    ├── build.rs
    ├── icons/...
    └── src/
        ├── main.rs             # window setup, command registration
        ├── backend.rs          # sidecar lifecycle (port, spawn, healthcheck, kill)
        └── commands.rs         # Tauri commands exposed to the JS layer
```

A small shared runtime hook is added to `frontend/src/lib/runtime.ts` and consumed by `frontend/src/api/index.ts`. The hook is a no-op in browser mode and returns desktop-provided values in Tauri mode.

## Components

### 1. Shared runtime hook (`frontend/src/lib/runtime.ts`)

Exports:

- `isDesktop(): boolean` — `true` when `window.__OMNILLM_DESKTOP__` is set by the Tauri preload.
- `getDesktopBackend(): { baseUrl: string; apiKey: string } | null` — reads `window.__OMNILLM_DESKTOP__` injected by the Tauri app at startup. Returns `null` outside Tauri.

This is the single seam that desktop mode uses to inject backend info. It does not depend on any Tauri APIs at compile time, so the browser build is unaffected.

### 2. API client (`frontend/src/api/index.ts`)

Modified `getBackendBase()` and `getApiKey()` to consult `getDesktopBackend()` first:

- If desktop info is present → use `baseUrl` and `apiKey` directly, skip auto-detection and meta-tag lookup.
- Otherwise → existing behavior unchanged (auto-detect, meta tag, `__API_KEY__`).

### 3. Desktop entrypoint (`desktop/`)

- Vite config aliases `@` → `../frontend/src` so all existing imports resolve into the shared codebase.
- `desktop/src/main.tsx`:
  - Reads the desktop bridge via the Tauri `invoke()` API: `desktop_backend_info()` → `{ baseUrl, apiKey, version }`.
  - Sets `window.__OMNILLM_DESKTOP__ = { baseUrl, apiKey }` before mounting the shared `App`.
- `desktop/index.html` mounts the same React tree as the browser build.

### 4. Tauri Rust layer (`desktop/src-tauri/src/`)

`backend.rs`:
- `pick_free_port() -> u16` — binds to `127.0.0.1:0`, returns the OS-assigned port.
- `load_or_create_api_key() -> String` — reads `~/.config/omnillm/desktop-api-key`, generates a new random key if absent, and persists it (mirrors how the existing CLI persists `~/.config/omnillm/api-key`).
- `spawn_sidecar(port, api_key) -> ManagedSidecar` — spawns `omniproxy start --port <port> --api-key <key> --host 127.0.0.1` as a Tauri sidecar (binary placed under `desktop/src-tauri/binaries/omniproxy-<target-triple>`). Returns a handle that owns the child process.
- `wait_for_healthz(port) -> Result<()>` — polls `http://127.0.0.1:<port>/healthz` with a 30-second timeout.
- On window close: send SIGTERM (Unix) / `kill` (Windows) to the sidecar with a graceful timeout, fall back to forced kill.

`commands.rs`:
- `#[tauri::command] fn desktop_backend_info() -> BackendInfo` — returns `{ baseUrl, apiKey, version }` to JS.

`main.rs`:
- On `setup`, run the lifecycle: pick port → load/create key → spawn sidecar → wait for `/healthz`.
- On failure, render an in-Rust fallback HTML page with logs and a “Retry” button.
- Register the `desktop_backend_info` command.

## Data Flow

Startup:

1. Tauri `setup` runs `backend::start()`:
   - Pick free port `P`.
   - Load/create API key `K`.
   - Spawn `omniproxy start --port P --api-key K --host 127.0.0.1`.
   - Poll `/healthz` until 200 or timeout.
2. Tauri opens main window pointed at `desktop/dist/index.html`.
3. JS calls `invoke("desktop_backend_info")` → `{ baseUrl: "http://127.0.0.1:P", apiKey: K }`.
4. JS sets `window.__OMNILLM_DESKTOP__` and mounts the shared `App`.
5. All API calls go through `baseUrl + path` with `Authorization: Bearer <apiKey>`.

Shutdown:
- Window close → Rust calls `child.kill_with_grace()` (SIGTERM with 5-second timeout, then SIGKILL).

## Error Handling

- Sidecar failed to spawn: Rust shows a fallback HTML page (separate from the React app) with the spawn error and a “Retry” button that re-runs `backend::start()`.
- Healthcheck timeout: same fallback page, with the last few lines of sidecar stderr captured to a buffer.
- Port collision (rare with OS-assigned port, but defensive): retry up to 3 times.
- Sidecar crash mid-session: Rust emits a `backend://crashed` Tauri event; the React UI shows a toast with a “Restart backend” action that calls a `restart_backend` command. (Optional follow-up; v1 may simply close the window.)

## Build & Packaging

- `make build-desktop` (new):
  1. Build the host-target `omniproxy` binary as `desktop/src-tauri/binaries/omniproxy-<target-triple>`.
  2. Build the shared frontend (`bun run build --outDir desktop/dist` via the desktop Vite config — does not touch the existing browser build under `pages/admin/`).
  3. Run `tauri build` from `desktop/`.
- `bun run desktop:dev`: runs `tauri dev`, which uses `vite` from `desktop/`. The sidecar is spawned in dev mode pointing at a debug-built `omniproxy` next to the dev binary. Hot-reload of the shared React code works because Vite watches `../frontend/src`.
- Existing scripts (`bun run dev`, `bun run build`, `make build-go`, `omnillm start`, `omniproxy start`) are untouched.

## Tests

- **Frontend unit:**
  - New `tests/frontend/desktop-runtime.test.ts` covering:
    - `isDesktop()` returns `false` without bridge, `true` with bridge.
    - `getDesktopBackend()` returns the bridge values when present.
    - `apiFetch` (or `getBackendBase`/`getApiKey`) routes through the bridge when set.
- **Rust unit:**
  - `pick_free_port()` returns a non-zero port that can be bound.
  - `load_or_create_api_key()` is idempotent.
- **Lint/typecheck/build:**
  - `bun run lint`, `bun run typecheck`, and `make build-go` must continue to pass.

Desktop end-to-end smoke testing via Playwright + `tauri-driver` is captured as a follow-up — v1 ships with the unit tests above plus a documented manual smoke (open app → see admin UI → switch tabs → close).

## Out of Scope

- Auto-update channel (Tauri updater can be added later).
- Code signing / notarization (manual for now).
- Tray icon, multi-window, OS deep links.
- Any feature divergence from the web admin UI.

## Risks & Mitigations

| Risk | Mitigation |
|---|---|
| Sidecar startup latency | Show a small splash inside Tauri until `/healthz` succeeds. |
| Cross-platform child-process kill semantics | Use Tauri’s `Command` API with explicit kill on app exit; add a hard-kill fallback. |
| Coupling shared frontend to desktop-only behavior | Keep the bridge to a single `lib/runtime.ts` module; never branch UI features on `isDesktop()`. |
| Build-time dependency on Rust toolchain in CI | Limit `make build-desktop` to platforms that have `cargo`; fall back to a no-op with a clear message otherwise. |
