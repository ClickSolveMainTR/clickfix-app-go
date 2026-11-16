//! İki uygulamada da aynı kurulan Tauri eklentileri.

use tauri::{Manager, Runtime, plugin::TauriPlugin};
use tauri_plugin_log::{Target, TargetKind};

/// Ana pencerenin etiketi.
pub const MAIN_WINDOW: &str = "main";

/// Tek örnek: ikinci açılış yeni süreç başlatmaz, var olan ana pencereyi öne
/// getirir. Eklenti zincirinde ilk sırada kaydedilmelidir.
pub fn single_instance<R: Runtime>() -> TauriPlugin<R> {
    tauri_plugin_single_instance::init(|app, args, _cwd| {
        log::info!("ikinci örnek açılmak istendi: {args:?}");
        if let Some(w) = app.get_webview_window(MAIN_WINDOW) {
            let _ = w.unminimize();
            let _ = w.show();
            let _ = w.set_focus();
        }
    })
}

/// Günlük: işletim sisteminin uygulama günlük dizinine (`<app>.log`, döner)
/// ve stdout'a yazar.
pub fn logging<R: Runtime>() -> TauriPlugin<R> {
    tauri_plugin_log::Builder::new()
        .level(log::LevelFilter::Info)
        .max_file_size(5 * 1024 * 1024)
        .rotation_strategy(tauri_plugin_log::RotationStrategy::KeepOne)
        .targets([
            Target::new(TargetKind::LogDir { file_name: None }),
            Target::new(TargetKind::Stdout),
        ])
        .build()
}
