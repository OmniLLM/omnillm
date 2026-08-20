// Sidecar lifecycle for the bundled `omnillm` backend.
//
// Responsibilities:
//   * pick a free localhost port
//   * load or create a per-install API key
//   * spawn the bundled `omnillm` binary
//   * poll /healthz until ready
//   * cleanly shut the child down on app exit
//
// The Tauri layer (commands.rs / main.rs) is thin glue around this module.

use std::io;
use std::net::TcpListener;
use std::path::PathBuf;
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

use rand::Rng;
use serde::Serialize;

#[derive(Debug, Clone, Serialize)]
pub struct BackendInfo {
    #[serde(rename = "baseUrl")]
    pub base_url: String,
    #[serde(rename = "apiKey")]
    pub api_key: String,
    pub version: String,
}

#[derive(Default)]
pub struct BackendState {
    pub info: Option<BackendInfo>,
    pub child: Option<Arc<Mutex<Option<std::process::Child>>>>,
}

pub type SharedBackendState = Arc<Mutex<BackendState>>;

/// Bind to 127.0.0.1:0 and return the OS-assigned port.
///
/// We drop the listener before returning so the spawned `omnillm` can
/// claim the port immediately. There is a tiny race window between drop
/// and child bind, which is acceptable here because the desktop app is
/// the only thing on the user's machine racing for that port.
pub fn pick_free_port() -> io::Result<u16> {
    let listener = TcpListener::bind(("127.0.0.1", 0))?;
    let port = listener.local_addr()?.port();
    drop(listener);
    Ok(port)
}

fn api_key_path() -> Option<PathBuf> {
    let mut p = dirs::config_dir()?;
    p.push("omnillm");
    p.push("desktop-api-key");
    Some(p)
}

fn random_api_key() -> String {
    // 32 hex chars; not security-grade but fine for a per-install local key.
    let mut rng = rand::thread_rng();
    let mut buf = [0u8; 16];
    rng.fill(&mut buf);
    let mut s = String::with_capacity(32);
    for b in buf.iter() {
        use std::fmt::Write;
        let _ = write!(s, "{:02x}", b);
    }
    s
}

pub fn load_or_create_api_key() -> io::Result<String> {
    let Some(path) = api_key_path() else {
        return Ok(random_api_key());
    };
    if let Ok(contents) = std::fs::read_to_string(&path) {
        let trimmed = contents.trim();
        if !trimmed.is_empty() {
            return Ok(trimmed.to_string());
        }
    }
    if let Some(parent) = path.parent() {
        std::fs::create_dir_all(parent)?;
    }
    let key = random_api_key();
    std::fs::write(&path, &key)?;
    Ok(key)
}

pub fn wait_for_healthz(port: u16, timeout: Duration) -> Result<(), String> {
    let deadline = Instant::now() + timeout;
    let url = format!("http://127.0.0.1:{}/healthz", port);
    let mut last_err = String::from("did not start");
    while Instant::now() < deadline {
        match ureq::get(&url).timeout(Duration::from_millis(800)).call() {
            Ok(resp) if resp.status() == 200 => return Ok(()),
            Ok(resp) => last_err = format!("status {}", resp.status()),
            Err(e) => last_err = format!("{}", e),
        }
        std::thread::sleep(Duration::from_millis(150));
    }
    Err(format!("backend healthcheck failed: {}", last_err))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn pick_free_port_returns_usable_port() {
        let port = pick_free_port().expect("pick port");
        assert!(port > 0);
        // Re-binding should succeed because pick_free_port drops the listener.
        let listener = TcpListener::bind(("127.0.0.1", port)).expect("rebind");
        drop(listener);
    }

    #[test]
    fn random_api_key_is_32_hex() {
        let key = random_api_key();
        assert_eq!(key.len(), 32);
        assert!(key.chars().all(|c| c.is_ascii_hexdigit()));
    }

    #[test]
    fn load_or_create_api_key_roundtrips() {
        // Override HOME / XDG_CONFIG_HOME for the duration of this test so
        // we don't touch the developer's real ~/.config.
        let tmp = std::env::temp_dir().join(format!(
            "omnillm-test-{}",
            std::process::id()
        ));
        std::fs::create_dir_all(&tmp).unwrap();
        let prev_xdg = std::env::var_os("XDG_CONFIG_HOME");
        let prev_home = std::env::var_os("HOME");
        std::env::set_var("XDG_CONFIG_HOME", &tmp);
        std::env::set_var("HOME", &tmp);

        let k1 = load_or_create_api_key().unwrap();
        let k2 = load_or_create_api_key().unwrap();
        assert_eq!(k1, k2);
        assert_eq!(k1.len(), 32);

        // Restore env
        match prev_xdg {
            Some(v) => std::env::set_var("XDG_CONFIG_HOME", v),
            None => std::env::remove_var("XDG_CONFIG_HOME"),
        }
        match prev_home {
            Some(v) => std::env::set_var("HOME", v),
            None => std::env::remove_var("HOME"),
        }
        let _ = std::fs::remove_dir_all(&tmp);
    }
}
