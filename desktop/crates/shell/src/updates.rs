//! Kendini güncelleme (CF-240).
//!
//! Tauri updater eklentisini kurar. Sürüm (release) derlemesinde açılışta
//! arka planda API'ye sorar (`plugins.updater.endpoints`, `tauri.conf.json`).
//! Yeni sürüm varsa paketi indirir ve imzasını `plugins.updater.pubkey` ile
//! doğrular; kurulum uygulama kapanırken yapılır. Böylece süren bir uzaktan
//! destek oturumu veya açık bir konuşma yeniden başlatmayla kesilmez; yeni
//! sürüm bir sonraki açılışta çalışır. Hata yalnız günlüğe yazılır.

use std::sync::Mutex;

use tauri::plugin::{Builder, TauriPlugin};
use tauri::{AppHandle, Manager, RunEvent, Runtime};
use tauri_plugin_updater::{Update, UpdaterExt};

/// İndirilmiş, imzası doğrulanmış, kapanışta kurulacak güncelleme.
#[derive(Default)]
struct Pending(Mutex<Option<(Update, Vec<u8>)>>);

/// Güncelleyici eklentisi. Uygulamalar bunu bir kez `.plugin(...)` ile
/// kaydeder; `tauri_plugin_updater` bu eklentinin kurulumunda eklenir.
pub fn plugin<R: Runtime>() -> TauriPlugin<R> {
    Builder::new("clickfix-updates")
        .setup(|app, _api| {
            app.plugin(tauri_plugin_updater::Builder::new().build())?;
            app.manage(Pending::default());
            if cfg!(debug_assertions) {
                log::info!("geliştirme derlemesi: güncelleme denetimi kapalı");
                return Ok(());
            }
            // Açık anahtar release iş akışında yazılır; yoksa paket
            // doğrulanamaz, sormanın anlamı yok.
            if !pubkey_set(app) {
                log::warn!("plugins.updater.pubkey boş: güncelleme denetimi kapalı");
                return Ok(());
            }
            let handle = app.clone();
            tauri::async_runtime::spawn(async move {
                if let Err(e) = fetch(&handle).await {
                    log::warn!("güncelleme denetimi başarısız: {e}");
                }
            });
            Ok(())
        })
        .on_event(|app, event| {
            if let RunEvent::Exit = event {
                install(app);
            }
        })
        .build()
}

fn pubkey_set<R: Runtime>(app: &AppHandle<R>) -> bool {
    app.config()
        .plugins
        .0
        .get("updater")
        .and_then(|u| u.get("pubkey"))
        .and_then(|v| v.as_str())
        .is_some_and(|k| !k.trim().is_empty())
}

async fn fetch<R: Runtime>(app: &AppHandle<R>) -> tauri_plugin_updater::Result<()> {
    let Some(update) = app.updater()?.check().await? else {
        log::info!("güncelleme yok");
        return Ok(());
    };
    log::info!(
        "yeni sürüm {} (şu an {}) indiriliyor",
        update.version,
        update.current_version
    );
    // download() imzayı doğrular; imzasız veya bozuk paket hata döner.
    let bytes = update.download(|_, _| {}, || {}).await?;
    if let Some(pending) = app.try_state::<Pending>()
        && let Ok(mut slot) = pending.0.lock()
    {
        *slot = Some((update, bytes));
        log::info!("güncelleme indirildi; uygulama kapanınca kurulacak");
    }
    Ok(())
}

fn install<R: Runtime>(app: &AppHandle<R>) {
    let Some(pending) = app.try_state::<Pending>() else {
        return;
    };
    let taken = pending.0.lock().ok().and_then(|mut slot| slot.take());
    if let Some((update, bytes)) = taken {
        // Windows'ta kurulum programı başlatılır ve süreç kapanır.
        match update.install(bytes) {
            Ok(()) => log::info!("sürüm {} kuruldu", update.version),
            Err(e) => log::warn!("güncelleme kurulamadı: {e}"),
        }
    }
}
