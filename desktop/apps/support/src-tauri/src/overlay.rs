//! Paylaşım göstergeleri (docs/design/masaustu.md §5 Akış, 8. adım):
//!
//! - **Çerçeve** (`frame`): birincil ekranı kaplayan şeffaf, tıklamaları
//!   geçiren, her zaman üstte pencere; kenarda yeşil çerçeve çizer.
//! - **Çubuk** (`bar`): her zaman üstte, sürüklenebilir durum çubuğu;
//!   Durdur ve Kontrolü geri al düğmeleri.
//! - **Acil durdurma**: `Ctrl+Alt+Shift+S` genel kısayolu yalnız paylaşım
//!   sürerken kayıtlıdır.
//!
//! Göstergeler sidecar'ın `state` olayından çizilir (`phase` `active` ya da
//! `reconnecting`, desktop/sidecar/IPC.md).
//! Böylece ana arayüz takılsa bile paylaşım sürdükçe gösterge kalır.

use serde_json::Value;
use tauri::{
    AppHandle, Emitter, LogicalPosition, LogicalSize, Manager, Monitor, Runtime, WebviewUrl,
    WebviewWindow, WebviewWindowBuilder,
};
use tauri_plugin_global_shortcut::GlobalShortcutExt;

pub const FRAME_WINDOW: &str = "frame";
pub const BAR_WINDOW: &str = "bar";
pub const OVERLAY_EVENT: &str = "support://overlay";
pub const EMERGENCY_SHORTCUT: &str = "Ctrl+Alt+Shift+KeyS";

const BAR_WIDTH: f64 = 800.0;
const BAR_HEIGHT: f64 = 92.0;

/// Durum ekran yayınının sürdüğünü söylüyor mu? Bağlantı yeniden kurulurken
/// de yakalama sürdüğü için gösterge kalır.
pub fn is_sharing(state: &Value) -> bool {
    matches!(
        state.get("phase").and_then(Value::as_str),
        Some("active" | "reconnecting")
    )
}

/// Sidecar durumunu göstergelere uygular: paylaşım varsa açar ve günceller,
/// yoksa kapatır.
pub fn apply<R: Runtime>(app: &AppHandle<R>, state: &Value) {
    if is_sharing(state) {
        if let Err(e) = show(app) {
            log::error!("paylaşım göstergesi açılamadı: {e}");
        }
        let _ = app.emit_to(BAR_WINDOW, OVERLAY_EVENT, state);
    } else {
        hide(app);
    }
}

fn show<R: Runtime>(app: &AppHandle<R>) -> tauri::Result<()> {
    let monitor = primary_monitor(app)?;
    if app.get_webview_window(FRAME_WINDOW).is_none() {
        build_frame(app, monitor.as_ref())?;
    }
    if app.get_webview_window(BAR_WINDOW).is_none() {
        build_bar(app, monitor.as_ref())?;
    }
    let gs = app.global_shortcut();
    if !gs.is_registered(EMERGENCY_SHORTCUT)
        && let Err(e) = gs.register(EMERGENCY_SHORTCUT)
    {
        log::error!("acil durdurma kısayolu kaydedilemedi: {e}");
    }
    Ok(())
}

/// Göstergeleri kapatır ve acil kısayolu bırakır.
pub fn hide<R: Runtime>(app: &AppHandle<R>) {
    for label in [BAR_WINDOW, FRAME_WINDOW] {
        if let Some(w) = app.get_webview_window(label)
            && let Err(e) = w.destroy()
        {
            log::warn!("{label} penceresi kapatılamadı: {e}");
        }
    }
    let gs = app.global_shortcut();
    if gs.is_registered(EMERGENCY_SHORTCUT)
        && let Err(e) = gs.unregister(EMERGENCY_SHORTCUT)
    {
        log::warn!("acil durdurma kısayolu bırakılamadı: {e}");
    }
}

fn primary_monitor<R: Runtime>(app: &AppHandle<R>) -> tauri::Result<Option<Monitor>> {
    let main = app.get_webview_window(clickfix_shell::plugins::MAIN_WINDOW);
    match main {
        Some(w) => w.primary_monitor(),
        None => app.primary_monitor(),
    }
}

/// Birincil ekranın mantıksal konumu ve boyutu.
fn monitor_rect(monitor: Option<&Monitor>) -> (f64, f64, f64, f64) {
    match monitor {
        Some(m) => {
            let scale = m.scale_factor();
            let pos: LogicalPosition<f64> = m.position().to_logical(scale);
            let size: LogicalSize<f64> = m.size().to_logical(scale);
            (pos.x, pos.y, size.width, size.height)
        }
        None => (0.0, 0.0, 1440.0, 900.0),
    }
}

fn overlay_window<'a, R: Runtime>(
    app: &'a AppHandle<R>,
    label: &str,
    page: &str,
) -> WebviewWindowBuilder<'a, R, AppHandle<R>> {
    WebviewWindowBuilder::new(app, label, WebviewUrl::App(page.into()))
        .title("ClickFix Destek")
        .decorations(false)
        .transparent(true)
        .shadow(false)
        .always_on_top(true)
        .visible_on_all_workspaces(true)
        .skip_taskbar(true)
        .resizable(false)
        .minimizable(false)
        .maximizable(false)
        .closable(false)
        // Göstergeler odak çalmaz; müşteri işine devam eder.
        .focused(false)
        // Temsilci göstergeleri görmez (sidecar yakalaması pencereyi
        // dışlamazsa işletim sistemi dışlar).
        .content_protected(true)
        .on_navigation(super::is_local_asset)
}

fn build_frame<R: Runtime>(
    app: &AppHandle<R>,
    monitor: Option<&Monitor>,
) -> tauri::Result<WebviewWindow<R>> {
    let (x, y, w, h) = monitor_rect(monitor);
    let win = overlay_window(app, FRAME_WINDOW, "frame.html")
        .focusable(false)
        .position(x, y)
        .inner_size(w, h)
        .build()?;
    win.set_ignore_cursor_events(true)?;
    Ok(win)
}

fn build_bar<R: Runtime>(
    app: &AppHandle<R>,
    monitor: Option<&Monitor>,
) -> tauri::Result<WebviewWindow<R>> {
    let (x, y, w, _) = monitor_rect(monitor);
    // macOS menü çubuğunun altında, Windows'ta ekranın üstünde.
    let top = if cfg!(target_os = "macos") {
        36.0
    } else {
        12.0
    };
    overlay_window(app, BAR_WINDOW, "bar.html")
        .position(x + ((w - BAR_WIDTH) / 2.0).max(0.0), y + top)
        .inner_size(BAR_WIDTH, BAR_HEIGHT)
        .build()
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn only_sharing_phase_shows_overlay() {
        assert!(is_sharing(&json!({"phase": "active"})));
        assert!(is_sharing(&json!({"phase": "reconnecting"})));
        for phase in ["idle", "resolved", "ready", "starting", "ending", "ended"] {
            assert!(!is_sharing(&json!({ "phase": phase })));
        }
        assert!(!is_sharing(&json!({})));
        assert!(!is_sharing(&Value::Null));
    }
}
