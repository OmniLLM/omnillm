# OmniLLM Desktop (Tauri)

A Tauri-based desktop wrapper around the existing OmniLLM admin UI.

The desktop app:

- launches the bundled `omnillm` binary as a sidecar on a free localhost port
- generates or reuses a per-install API key under `~/.config/omnillm/desktop-api-key`
- mounts the same React admin UI from `../frontend/src` inside a WebView
- routes all API calls through the local sidecar via a runtime bridge
  (`window.__OMNILLM_DESKTOP__`)

The Go backend is **also** still usable as a standalone binary:

- `omnillm start`
- packaged-runtime mode (port 5000 serving `/admin/`)
- dev mode (Go on 5002 + Vite on 5080)

## Develop

```sh
make desktop-dev
```

This builds the host-target `omnillm` binary into
`desktop/src-tauri/binaries/omnillm-<rust-host-triple>` and runs `tauri dev`.

## Build

```sh
make build-desktop
```

Outputs platform-native installers under `desktop/src-tauri/target/release/bundle/`.

## Layout

```
desktop/
├── package.json              # Tauri / Vite scripts only
├── vite.config.ts            # Aliases @ → ../frontend/src
├── index.html
├── src/
│   ├── main.tsx              # mounts shared App, sets bridge
│   └── desktop/desktop.css   # window-chrome polish only
└── src-tauri/
    ├── tauri.conf.json
    ├── Cargo.toml
    └── src/
        ├── main.rs           # window setup, sidecar boot
        ├── backend.rs        # port pick, api key, healthcheck
        └── commands.rs       # Tauri commands
```
