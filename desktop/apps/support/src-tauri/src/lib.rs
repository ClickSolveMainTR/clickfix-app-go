//! ClickFix Destek (müşteri hızlı destek uygulaması).
//!
//! Arayüz pakete gömülü statik bir Vite + React uygulamasıdır; uzak kök
//! yüklemez. Ağ çağrıları ileride (CF-238, CF-239) sidecar üzerinden yapılır.

use clickfix_shell::plugins::{self, MAIN_WINDOW};
use clickfix_shell::{Sidecar, SidecarSpec};
use serde_json::{Value, json};
use tauri::{Manager, RunEvent, State, WebviewUrl, WebviewWindowBuilder};

/// Sidecar'ın `version` yanıtını arayüze iletir.
#[tauri::command]
async fn sidecar_version(sidecar: State<'_, Sidecar>) -> Result<Value, String> {
    sidecar
        .call("version", json!({}))
        .await
        .map_err(|e| e.to_string())
}

pub fn run() {
    let app = tauri::Builder::default()
        .plugin(plugins::single_instance())
        .plugin(plugins::logging())
        .invoke_handler(tauri::generate_handler![sidecar_version])
        .setup(|app| {
            log::info!("ClickFix Destek başlıyor");
            WebviewWindowBuilder::new(app, MAIN_WINDOW, WebviewUrl::App("index.html".into()))
                .title("ClickFix Destek")
                .inner_size(480.0, 640.0)
                .min_inner_size(400.0, 560.0)
                .resizable(true)
                // Yerel arayüzden dışarı gezinme yok.
                .on_navigation(|url| {
                    let ok = matches!(url.scheme(), "tauri")
                        || url.host_str() == Some("tauri.localhost")
                        || (cfg!(debug_assertions) && url.host_str() == Some("127.0.0.1"));
                    if !ok {
                        log::warn!("gezinme engellendi: {url}");
                    }
                    ok
                })
                .build()?;

            let spec = SidecarSpec::bundled("support")?;
            app.manage(Sidecar::spawn(spec));
            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("ClickFix Destek başlatılamadı");

    app.run(|handle, event| {
        if let RunEvent::Exit = event
            && let Some(sc) = handle.try_state::<Sidecar>()
        {
            sc.shutdown();
        }
    });
}
