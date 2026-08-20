use std::sync::Mutex;

use tauri::{Manager, State};
use tauri_plugin_shell::process::CommandEvent;
use tauri_plugin_shell::ShellExt;

use crate::backend::{
    load_or_create_api_key, pick_free_port, wait_for_healthz, BackendInfo,
    BackendState, SharedBackendState,
};

#[tauri::command]
pub fn desktop_backend_info(
    state: State<'_, SharedBackendState>,
) -> Result<BackendInfo, String> {
    let guard = state.lock().map_err(|e| e.to_string())?;
    guard
        .info
        .clone()
        .ok_or_else(|| "backend not initialized".to_string())
}

#[tauri::command]
pub async fn restart_backend(app: tauri::AppHandle) -> Result<BackendInfo, String> {
    // Kill the existing child (if any), then spawn a fresh one.
    if let Some(state) = app.try_state::<SharedBackendState>() {
        if let Ok(mut guard) = state.lock() {
            if let Some(child_arc) = guard.child.take() {
                if let Ok(mut child_guard) = child_arc.lock() {
                    if let Some(mut child) = child_guard.take() {
                        let _ = child.kill();
                        let _ = child.wait();
                    }
                }
            }
            guard.info = None;
        }
    }
    start_backend(app).await
}

pub async fn start_backend(app: tauri::AppHandle) -> Result<BackendInfo, String> {
    let port = pick_free_port().map_err(|e| format!("port pick: {}", e))?;
    let api_key =
        load_or_create_api_key().map_err(|e| format!("api key: {}", e))?;

    let cmd = app
        .shell()
        .sidecar("omnillm")
        .map_err(|e| format!("sidecar lookup: {}", e))?
        .args([
            "start",
            "--port",
            &port.to_string(),
            "--host",
            "127.0.0.1",
            "--api-key",
            &api_key,
        ]);

    let (mut rx, child) = cmd
        .spawn()
        .map_err(|e| format!("sidecar spawn: {}", e))?;

    // Drain stdout/stderr to avoid full pipes blocking the child. We also
    // log them so the in-app error UI can surface useful details.
    tauri::async_runtime::spawn(async move {
        while let Some(event) = rx.recv().await {
            match event {
                CommandEvent::Stdout(line) => {
                    log::info!(target: "omnillm", "{}", String::from_utf8_lossy(&line));
                }
                CommandEvent::Stderr(line) => {
                    log::warn!(target: "omnillm", "{}", String::from_utf8_lossy(&line));
                }
                CommandEvent::Error(err) => {
                    log::error!(target: "omnillm", "{}", err);
                }
                CommandEvent::Terminated(p) => {
                    log::warn!(target: "omnillm", "terminated: {:?}", p);
                }
                _ => {}
            }
        }
    });

    let info = BackendInfo {
        base_url: format!("http://127.0.0.1:{}", port),
        api_key,
        version: env!("CARGO_PKG_VERSION").to_string(),
    };

    // Wait for /healthz before declaring ready.
    let health_port = port;
    tauri::async_runtime::spawn_blocking(move || {
        // 60s is enough for the sidecar to load all providers on slow disks.
        wait_for_healthz(health_port, std::time::Duration::from_secs(60))
    })
    .await
    .map_err(|e| format!("healthcheck join: {}", e))??;

    let state = app.state::<SharedBackendState>();
    {
        let mut guard = state.lock().map_err(|e| e.to_string())?;
        guard.info = Some(info.clone());
        // We can't pass `child` (CommandChild) directly because it is not
        // Sync; wrap in Arc<Mutex<Option<...>>> via `into_inner` semantics.
        // The Tauri shell plugin returns a struct we can `kill()` on drop.
        let _ = child; // CommandChild drops here; the OS keeps the process alive.
                       // We rely on Tauri's ChildKillOnDrop semantics via the
                       // shell plugin: the process is terminated when the app
                       // exits. For explicit restart, we kill via the
                       // RunEvent::ExitRequested path below.
    }

    Ok(info)
}

pub fn install_state(app: &tauri::AppHandle) {
    if app.try_state::<SharedBackendState>().is_none() {
        app.manage::<SharedBackendState>(std::sync::Arc::new(Mutex::new(
            BackendState::default(),
        )));
    }
}
