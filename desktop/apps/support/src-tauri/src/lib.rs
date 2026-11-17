//! ClickFix Destek (müşteri hızlı destek uygulaması), docs/design/masaustu.md §5, §7.
//!
//! Arayüz pakete gömülü statik bir Vite + React uygulamasıdır; uzak kök
//! yüklemez. Ağ, LiveKit, ekran yakalama ve girdi enjeksiyonu Go sidecar'ın
//! (`--mode=support`, CF-238) işidir. Bu kabuk:
//!
//! - arayüzün sidecar yöntemlerini yalnız izin listesindekilerle çağırmasına
//!   izin verir ([`sidecar_call`]) ve sidecar olaylarını arayüze iletir;
//! - `clickfix-support://join?t=...` derin bağlantısını alır ([`links`]);
//! - paylaşım sürerken ekran kenarı çerçevesini, sürüklenebilir durum
//!   çubuğunu ve `Ctrl+Alt+Shift+S` acil durdurmasını yönetir ([`overlay`]);
//! - Windows'ta WebView2 yoksa pencere açmadan açıklar ([`webview2`]).
//!
//! Otomatik başlatma ve tepsi yoktur; ana pencere kapanınca oturum durdurulur,
//! sidecar sonlandırılır ve uygulama çıkar.

mod links;
mod overlay;
#[cfg(target_os = "windows")]
mod webview2;

use std::sync::Mutex;
use std::time::Duration;

use clickfix_shell::plugins::{self, MAIN_WINDOW};
use clickfix_shell::{CallError, Sidecar, SidecarSpec};
use serde::Serialize;
use serde_json::{Value, json};
use tauri::{
    AppHandle, Emitter, Manager, RunEvent, State, WebviewUrl, WebviewWindowBuilder, WindowEvent,
};
use tauri_plugin_deep_link::DeepLinkExt;
use tauri_plugin_global_shortcut::ShortcutState;
use url::Url;

/// Arayüzün çağırabileceği sidecar yöntemleri (desktop/sidecar/IPC.md).
/// `sysinfo.get` henüz kullanılmadığı için listede yok.
const SIDECAR_METHODS: &[&str] = &[
    "session.resolve",
    "session.consent",
    "session.start",
    "session.stop",
    "session.report",
    "control.revoke",
    "permissions.status",
    "permissions.request",
];

const SIDECAR_EVENT: &str = "support://sidecar";
const JOIN_EVENT: &str = "support://join";
const BAR_ACTION_EVENT: &str = "support://bar-action";
/// Kapanışta oturumu durdurmak için beklenen en uzun süre.
const STOP_ON_EXIT_TIMEOUT: Duration = Duration::from_secs(3);
/// API çağıran yöntemler yeniden denemelerle 15 sn'ye kadar sürer (IPC.md
/// "Zaman aşımı"); kabuğun 5 sn varsayılanı yetmez.
const SIDECAR_CALL_TIMEOUT: Duration = Duration::from_secs(20);

#[derive(Default)]
struct Support {
    /// Soğuk açılışta gelen, arayüzün henüz almadığı katılma token'ı.
    join_token: Mutex<Option<String>>,
    /// Paylaşım sürerken son `state` olayı (çubuk penceresi açılışta okur).
    overlay: Mutex<Option<Value>>,
}

fn lock<T>(m: &Mutex<T>) -> std::sync::MutexGuard<'_, T> {
    m.lock().unwrap_or_else(std::sync::PoisonError::into_inner)
}

/// Arayüze dönen hata: `code` makinece okunur (IPC.md hata türü ya da kabuk
/// kodu), `message` sidecar'ın müşteriye gösterilebilir metnidir.
#[derive(Debug, Serialize)]
struct IpcError {
    code: String,
    message: String,
}

impl IpcError {
    fn new(code: &str, message: impl Into<String>) -> Self {
        Self {
            code: code.into(),
            message: message.into(),
        }
    }
}

impl From<CallError> for IpcError {
    fn from(e: CallError) -> Self {
        match e {
            CallError::Rpc { code, message } => match error_kind(code) {
                Some(kind) => Self::new(kind, message),
                None => Self::new(&format!("RPC_{code}"), message),
            },
            CallError::NotRunning | CallError::Closed => Self::new("NOT_RUNNING", e.to_string()),
            CallError::Timeout => Self::new("TIMEOUT", e.to_string()),
            CallError::Io(m) => Self::new("IPC", m),
        }
    }
}

/// IPC.md "Hata kodları": kabuk yalnız `code` ve `message`'ı taşır, arayüz
/// türe göre dallanır.
fn error_kind(code: i64) -> Option<&'static str> {
    Some(match code {
        -32602 => "INVALID_PARAMS",
        -32603 => "INTERNAL",
        -32001 => "CODE_INVALID",
        -32002 => "RATE_LIMITED",
        -32003 => "ENDED",
        -32004 => "UNAVAILABLE",
        -32005 => "NETWORK",
        -32006 => "PERMISSION_REQUIRED",
        -32007 => "UNSUPPORTED",
        -32008 => "INVALID_STATE",
        -32009 => "SERVER",
        -32010 => "UNAUTHORIZED",
        -32011 => "CONFLICT",
        -32012 => "CAPTURE_FAILED",
        -32013 => "LIVEKIT",
        _ => return None,
    })
}

/// Sidecar sürüm bilgisi (alt bilgi ve tanı).
#[tauri::command]
async fn sidecar_version(sidecar: State<'_, Sidecar>) -> Result<Value, IpcError> {
    Ok(sidecar.call("version", json!({})).await?)
}

/// İzin listesindeki bir sidecar yöntemini çağırır.
#[tauri::command]
async fn sidecar_call(
    sidecar: State<'_, Sidecar>,
    method: String,
    params: Value,
) -> Result<Value, IpcError> {
    if !SIDECAR_METHODS.contains(&method.as_str()) {
        return Err(IpcError::new(
            "METHOD_NOT_ALLOWED",
            format!("izin verilmeyen yöntem: {method}"),
        ));
    }
    Ok(sidecar.call(&method, params).await?)
}

/// Soğuk açılıştaki katılma token'ını bir kez verir.
#[tauri::command]
fn take_join_token(support: State<'_, Support>) -> Option<String> {
    lock(&support.join_token).take()
}

/// macOS Sistem Ayarları'nın ilgili gizlilik bölmesini açar.
#[tauri::command]
fn open_permission_settings(pane: String) -> Result<(), IpcError> {
    let url = match pane.as_str() {
        "screen_recording" => {
            "x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture"
        }
        "accessibility" => {
            "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"
        }
        _ => return Err(IpcError::new("BAD_PANE", pane)),
    };
    if !cfg!(target_os = "macos") {
        return Ok(());
    }
    tauri_plugin_opener::open_url(url, None::<&str>)
        .map_err(|e| IpcError::new("OPEN_FAILED", e.to_string()))
}

/// Sabit bağlantılar; arayüz rastgele adres açtıramaz.
#[tauri::command]
fn open_link(link: String) -> Result<(), IpcError> {
    let url = match link.as_str() {
        "privacy" => "https://getclickfix.com/legal/data-protection",
        "help" => "https://getclickfix.com/indir?app=support",
        "webview2" => "https://go.microsoft.com/fwlink/p/?LinkId=2124703",
        _ => return Err(IpcError::new("BAD_LINK", link)),
    };
    tauri_plugin_opener::open_url(url, None::<&str>)
        .map_err(|e| IpcError::new("OPEN_FAILED", e.to_string()))
}

/// İzin verildikten sonra (macOS Ekran Kaydı) uygulamayı yeniden başlatır.
#[tauri::command]
fn relaunch(app: AppHandle) {
    if let Some(sc) = app.try_state::<Sidecar>() {
        sc.shutdown();
    }
    app.restart();
}

/// Bitiş ekranındaki "Kapat".
#[tauri::command]
fn quit(app: AppHandle) {
    app.exit(0);
}

/// Çubuk penceresi açılınca son durumu okur.
#[tauri::command]
fn overlay_state(support: State<'_, Support>) -> Option<Value> {
    lock(&support.overlay).clone()
}

/// Çubuktaki Durdur / Kontrolü geri al.
#[tauri::command]
async fn bar_action(app: AppHandle, action: String) -> Result<(), IpcError> {
    match action.as_str() {
        "stop" | "revoke" => {
            perform_action(&app, &action).await;
            Ok(())
        }
        _ => Err(IpcError::new("BAD_ACTION", action)),
    }
}

/// Sahte IPC ile (yalnız geliştirme derlemesi) göstergeleri sürer.
#[tauri::command]
fn mock_overlay(app: AppHandle, state: Value) -> Result<(), IpcError> {
    if !cfg!(debug_assertions) {
        return Err(IpcError::new(
            "NOT_AVAILABLE",
            "yalnız geliştirme derlemesi",
        ));
    }
    apply_state(&app, &state);
    Ok(())
}

/// Durdurma ve kontrolü geri alma doğrudan sidecar'a gider (ana arayüz
/// takılsa da çalışır). Sidecar yöntemi bilmiyorsa (sahte IPC) istek ana
/// pencereye iletilir ve orada uygulanır.
async fn perform_action(app: &AppHandle, action: &str) {
    let method = if action == "revoke" {
        "control.revoke"
    } else {
        "session.stop"
    };
    let handled = match app.try_state::<Sidecar>() {
        Some(sc) => match sc.call(method, json!({})).await {
            Ok(_) => true,
            Err(e) => {
                log::warn!("{method} sidecar'da uygulanamadı: {e}");
                false
            }
        },
        None => false,
    };
    let _ = app.emit_to(
        MAIN_WINDOW,
        BAR_ACTION_EVENT,
        json!({ "action": action, "handled": handled }),
    );
}

/// Sidecar `state` olayı: göstergeleri günceller.
fn apply_state(app: &AppHandle, state: &Value) {
    let support = app.state::<Support>();
    *lock(&support.overlay) = overlay::is_sharing(state).then(|| state.clone());
    overlay::apply(app, state);
}

fn forward_events(app: AppHandle, sidecar: &Sidecar) {
    let mut rx = sidecar.subscribe();
    tauri::async_runtime::spawn(async move {
        use tokio::sync::broadcast::error::RecvError;
        loop {
            match rx.recv().await {
                Ok(ev) => {
                    match ev.event.as_str() {
                        "state" => apply_state(&app, &ev.params),
                        // Sidecar yeniden başladıysa paylaşım bitmiştir.
                        "ready" => apply_state(&app, &json!({ "phase": "idle" })),
                        _ => {}
                    }
                    let _ = app.emit_to(
                        MAIN_WINDOW,
                        SIDECAR_EVENT,
                        json!({ "event": ev.event, "params": ev.params }),
                    );
                }
                Err(RecvError::Lagged(n)) => log::warn!("{n} sidecar olayı kaçırıldı"),
                Err(RecvError::Closed) => break,
            }
        }
    });
}

/// `clickfix-support://` bağlantıları (soğuk açılış, ikinci örnek, macOS olayı).
fn handle_links(app: &AppHandle, urls: Vec<Url>, cold: bool) {
    for url in urls {
        let Some(token) = links::join_token(&url) else {
            log::warn!("tanınmayan bağlantı yok sayıldı");
            continue;
        };
        if let Some(w) = app.get_webview_window(MAIN_WINDOW) {
            let _ = w.unminimize();
            let _ = w.show();
            let _ = w.set_focus();
        }
        if cold {
            *lock(&app.state::<Support>().join_token) = Some(token);
        } else {
            let _ = app.emit_to(MAIN_WINDOW, JOIN_EVENT, token);
        }
    }
}

/// Ana pencere kapanırken: paylaşım sürüyorsa önce durdur, sonra çık.
fn close_main(app: &AppHandle) {
    let app = app.clone();
    tauri::async_runtime::spawn(async move {
        let sharing = lock(&app.state::<Support>().overlay).is_some();
        if sharing && let Some(sc) = app.try_state::<Sidecar>() {
            let stop = sc.call("session.stop", json!({}));
            if tokio::time::timeout(STOP_ON_EXIT_TIMEOUT, stop)
                .await
                .is_err()
            {
                log::warn!("kapanışta oturum durdurulamadı (zaman aşımı)");
            }
        }
        overlay::hide(&app);
        app.exit(0);
    });
}

/// Yerel arayüz dışındaki adresler engellenir.
pub(crate) fn is_local_asset(url: &Url) -> bool {
    match url.scheme() {
        "tauri" => true,
        // Windows yerel varlıkları http://tauri.localhost'tan sunar.
        "http" | "https" => {
            url.host_str() == Some("tauri.localhost")
                || (cfg!(debug_assertions) && url.host_str() == Some("127.0.0.1"))
        }
        _ => false,
    }
}

pub fn run() {
    #[cfg(target_os = "windows")]
    if webview2::explain_if_missing() {
        return;
    }

    let app = tauri::Builder::default()
        // Tek örnek ilk sırada; `deep-link` özelliğiyle ikinci açılışın
        // bağlantısını derin bağlantı eklentisine iletir.
        .plugin(plugins::single_instance())
        .plugin(tauri_plugin_deep_link::init())
        .plugin(plugins::logging())
        .plugin(clickfix_shell::updates::plugin())
        .plugin(
            tauri_plugin_global_shortcut::Builder::new()
                .with_handler(|app, _shortcut, event| {
                    if event.state == ShortcutState::Pressed {
                        log::info!("acil durdurma kısayolu");
                        let app = app.clone();
                        tauri::async_runtime::spawn(async move {
                            perform_action(&app, "emergency").await;
                        });
                    }
                })
                .build(),
        )
        .manage(Support::default())
        .invoke_handler(tauri::generate_handler![
            sidecar_version,
            sidecar_call,
            take_join_token,
            open_permission_settings,
            open_link,
            relaunch,
            quit,
            overlay_state,
            bar_action,
            mock_overlay
        ])
        .setup(|app| {
            log::info!("ClickFix Destek başlıyor");
            let handle = app.handle().clone();
            let main =
                WebviewWindowBuilder::new(app, MAIN_WINDOW, WebviewUrl::App("index.html".into()))
                    .title("ClickFix Destek")
                    .inner_size(460.0, 660.0)
                    .min_inner_size(420.0, 600.0)
                    .resizable(true)
                    .maximizable(false)
                    .on_navigation(|url| {
                        let ok = is_local_asset(url);
                        if !ok {
                            log::warn!("gezinme engellendi: {url}");
                        }
                        ok
                    })
                    .build()?;
            let close_app = handle.clone();
            main.on_window_event(move |event| {
                if let WindowEvent::CloseRequested { api, .. } = event {
                    api.prevent_close();
                    close_main(&close_app);
                }
            });

            // Derin bağlantılar: geliştirmede şemayı çalışma anında kaydet;
            // macOS'ta Info.plist, Windows'ta ilk açılış kaydı yapar.
            #[cfg(any(target_os = "linux", target_os = "windows"))]
            if let Err(e) = app.deep_link().register_all() {
                log::warn!("clickfix-support:// şeması kaydedilemedi: {e}");
            }
            let link_app = handle.clone();
            app.deep_link()
                .on_open_url(move |event| handle_links(&link_app, event.urls(), false));
            if let Ok(Some(urls)) = app.deep_link().get_current() {
                handle_links(&handle, urls, true);
            }

            let mut spec = SidecarSpec::bundled("support")?;
            spec.call_timeout = SIDECAR_CALL_TIMEOUT;
            let sidecar = Sidecar::spawn(spec);
            forward_events(handle.clone(), &sidecar);
            app.manage(sidecar);
            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("ClickFix Destek başlatılamadı");

    app.run(|handle, event| {
        if let RunEvent::Exit = event {
            overlay::hide(handle);
            if let Some(sc) = handle.try_state::<Sidecar>() {
                sc.shutdown();
            }
        }
    });
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn rpc_error_carries_backend_code() {
        let e = IpcError::from(CallError::Rpc {
            code: -32001,
            message: "Kod geçersiz".into(),
        });
        assert_eq!(e.code, "CODE_INVALID");
        assert_eq!(e.message, "Kod geçersiz");

        let e = IpcError::from(CallError::Rpc {
            code: -32601,
            message: "bilinmeyen yöntem: session.start".into(),
        });
        assert_eq!(e.code, "RPC_-32601");

        assert_eq!(IpcError::from(CallError::NotRunning).code, "NOT_RUNNING");
        assert_eq!(IpcError::from(CallError::Timeout).code, "TIMEOUT");
    }

    #[test]
    fn method_allowlist_matches_contract() {
        assert!(SIDECAR_METHODS.contains(&"session.resolve"));
        assert!(SIDECAR_METHODS.contains(&"permissions.request"));
        assert!(!SIDECAR_METHODS.contains(&"version"));
        assert!(!SIDECAR_METHODS.contains(&"input.inject"));
    }

    #[test]
    fn local_assets_only() {
        let ok = |s: &str| is_local_asset(&Url::parse(s).expect("adres"));
        assert!(ok("tauri://localhost/index.html"));
        assert!(ok("http://tauri.localhost/bar.html"));
        assert!(!ok("https://getclickfix.com/"));
        assert!(!ok("file:///etc/passwd"));
    }
}
