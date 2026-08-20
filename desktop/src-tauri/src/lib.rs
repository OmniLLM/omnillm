// OmniLLM desktop entrypoint (library form for Tauri v2).
//
// Starts the bundled `omnillm` sidecar, waits for it to be healthy, then
// exposes its baseUrl + api key to the WebView via the
// `desktop_backend_info` Tauri command. The shared React admin UI mounts
// after that and behaves identically to the browser build.

mod backend;
mod commands;

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let _ = env_logger::try_init();

    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .invoke_handler(tauri::generate_handler![
            commands::desktop_backend_info,
            commands::restart_backend,
        ])
        .setup(|app| {
            commands::install_state(&app.handle());
            let handle = app.handle().clone();
            tauri::async_runtime::spawn(async move {
                match commands::start_backend(handle.clone()).await {
                    Ok(info) => {
                        log::info!("backend ready at {}", info.base_url);
                    }
                    Err(e) => {
                        log::error!("backend failed to start: {}", e);
                    }
                }
            });
            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
