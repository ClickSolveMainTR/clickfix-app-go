//! ClickFix Masaüstü (temsilci uygulaması).
//!
//! Ana pencere önce paketlenmiş yerel sayfayı (`ui/index.html`) açar. Sayfa
//! `connect` komutunu çağırır; uzak kök (`CLICKFIX_APP_URL`, varsayılan
//! https://getclickfix.com) erişilebilirse pencere oraya gider, değilse yerel
//! sayfa hata ve "Yeniden dene" gösterir. Uzak köke IPC yalnız çalışma anında
//! eklenen `remote-app` capability'si ile, yalnız o kök için açılır.

use std::time::Duration;

use clickfix_shell::plugins::{self, MAIN_WINDOW};
use clickfix_shell::{AppConfig, Sidecar, SidecarSpec, net};
use tauri::ipc::CapabilityBuilder;
use tauri::webview::PageLoadEvent;
use tauri::{AppHandle, Manager, RunEvent, State, WebviewUrl, WebviewWindowBuilder};
use url::Url;

const CONNECT_TIMEOUT: Duration = Duration::from_secs(5);

/// Yerel sayfanın çağırdığı tek komut: uzak köke erişilebiliyorsa ana
/// pencereyi oraya yönlendirir, değilse hata metnini döndürür.
#[tauri::command]
async fn connect(app: AppHandle, config: State<'_, AppConfig>) -> Result<String, String> {
    let url = config.app_url.clone();
    let probe = url.clone();
    tauri::async_runtime::spawn_blocking(move || net::reachable(&probe, CONNECT_TIMEOUT))
        .await
        .map_err(|e| e.to_string())??;
    let window = app
        .get_webview_window(MAIN_WINDOW)
        .ok_or("ana pencere bulunamadı")?;
    window.navigate(url.clone()).map_err(|e| e.to_string())?;
    Ok(url.to_string())
}

/// Ana pencere yalnız yerel sayfaya ve uygulama köküne gidebilir; diğer
/// adresler engellenir (dış bağlantılar CF-236'da sistem tarayıcısında açılır).
fn navigation_allowed(config: &AppConfig, url: &Url) -> bool {
    config.is_app_origin(url) || is_local_asset(url)
}

fn is_local_asset(url: &Url) -> bool {
    match url.scheme() {
        "tauri" => true,
        // Windows ve Android yerel varlıkları http(s)://tauri.localhost'tan sunar.
        "http" | "https" => url.host_str() == Some("tauri.localhost"),
        _ => false,
    }
}

pub fn run() {
    let config = match AppConfig::from_env() {
        Ok(c) => c,
        Err(e) => {
            eprintln!("clickfix-agent: {e}");
            std::process::exit(2);
        }
    };

    let app = tauri::Builder::default()
        .plugin(plugins::single_instance())
        .plugin(plugins::logging())
        .plugin(clickfix_shell::updates::plugin())
        .manage(config.clone())
        .invoke_handler(tauri::generate_handler![connect])
        .setup(move |app| {
            log::info!("ClickFix Masaüstü başlıyor, kök {}", config.app_url);

            // Uzak köke IPC: yalnız bu kök, yalnız ana pencere. Şimdilik hiçbir
            // komut açılmaz; köprü komutları CF-236 ile eklenir.
            app.add_capability(
                CapabilityBuilder::new("remote-app")
                    .remote(config.remote_pattern())
                    .window(MAIN_WINDOW)
                    .permission("core:app:allow-version"),
            )?;

            let nav = config.clone();
            WebviewWindowBuilder::new(app, MAIN_WINDOW, WebviewUrl::App("index.html".into()))
                .title("ClickFix Masaüstü")
                .inner_size(1280.0, 800.0)
                .min_inner_size(900.0, 600.0)
                .on_navigation(move |url| {
                    let ok = navigation_allowed(&nav, url);
                    if !ok {
                        log::warn!("gezinme engellendi: {url}");
                    }
                    ok
                })
                .on_page_load(|_, payload| {
                    if payload.event() == PageLoadEvent::Finished {
                        log::info!("sayfa yüklendi: {}", payload.url());
                    }
                })
                .build()?;

            match SidecarSpec::bundled("agent") {
                Ok(spec) => {
                    app.manage(Sidecar::spawn(spec));
                }
                Err(e) => log::error!("sidecar yolu bulunamadı: {e}"),
            }
            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("ClickFix Masaüstü başlatılamadı");

    app.run(|handle, event| {
        if let RunEvent::Exit = event
            && let Some(sc) = handle.try_state::<Sidecar>()
        {
            sc.shutdown();
        }
    });
}

#[cfg(test)]
mod tests {
    use super::*;

    fn url(s: &str) -> Url {
        Url::parse(s).expect("url")
    }

    #[test]
    fn navigation_policy() {
        let c = AppConfig::from_value(None).expect("yapılandırma");
        assert!(navigation_allowed(
            &c,
            &url("https://getclickfix.com/inbox")
        ));
        assert!(navigation_allowed(&c, &url("tauri://localhost/index.html")));
        assert!(navigation_allowed(
            &c,
            &url("http://tauri.localhost/index.html")
        ));
        assert!(!navigation_allowed(
            &c,
            &url("https://accounts.google.com/")
        ));
        assert!(!navigation_allowed(
            &c,
            &url("https://getclickfix.com.evil.test/")
        ));
        assert!(!navigation_allowed(&c, &url("file:///etc/passwd")));
    }
}
