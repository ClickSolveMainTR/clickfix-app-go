//! Kullanıcı ayarları: uygulama yapılandırma dizininde `settings.json`.
//!
//! Tepsi menüsü `close_to_tray` ve `autostart`'ı değiştirir. Genel kısayol
//! şimdilik yalnız bu dosyadan değiştirilir (boş değer kısayolu kapatır);
//! geçersiz değer günlüğe yazılır ve varsayılan kullanılır.

use std::path::{Path, PathBuf};

use serde::{Deserialize, Serialize};

pub const FILE_NAME: &str = "settings.json";

/// Pencereyi öne getirip Cmd+K paletini açan varsayılan kısayol.
pub const DEFAULT_SHORTCUT: &str = "CommandOrControl+Shift+K";

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(default)]
pub struct Settings {
    /// Pencereyi kapatmak uygulamayı tepsiye küçültür.
    pub close_to_tray: bool,
    /// İşletim sistemi oturumu açılınca başlat (varsayılan kapalı).
    pub autostart: bool,
    /// Genel kısayol (Tauri biçimi, ör. `CommandOrControl+Shift+K`).
    pub shortcut: String,
}

impl Default for Settings {
    fn default() -> Self {
        Self {
            close_to_tray: true,
            autostart: false,
            shortcut: DEFAULT_SHORTCUT.to_owned(),
        }
    }
}

impl Settings {
    /// Dosyayı okur; yoksa ya da bozuksa varsayılanlar.
    pub fn load(path: &Path) -> Self {
        match std::fs::read_to_string(path) {
            Ok(text) => serde_json::from_str(&text).unwrap_or_else(|e| {
                log::warn!("ayar dosyası okunamadı, varsayılanlar kullanılıyor: {e}");
                Self::default()
            }),
            Err(_) => Self::default(),
        }
    }

    pub fn save(&self, path: &Path) -> std::io::Result<()> {
        if let Some(dir) = path.parent() {
            std::fs::create_dir_all(dir)?;
        }
        let text = serde_json::to_string_pretty(self).map_err(std::io::Error::other)?;
        // Yarım yazılmış dosya kalmasın: önce geçici dosya, sonra yeniden adlandır.
        let tmp = path.with_extension("json.tmp");
        std::fs::write(&tmp, text)?;
        std::fs::rename(&tmp, path)
    }
}

/// Ayar dosyasının yolu ve son okunan değer.
pub struct SettingsStore {
    path: PathBuf,
    value: std::sync::Mutex<Settings>,
}

impl SettingsStore {
    pub fn open(dir: &Path) -> Self {
        let path = dir.join(FILE_NAME);
        let value = Settings::load(&path);
        Self {
            path,
            value: std::sync::Mutex::new(value),
        }
    }

    pub fn get(&self) -> Settings {
        self.value
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .clone()
    }

    /// Değiştirir ve diske yazar.
    pub fn update(&self, f: impl FnOnce(&mut Settings)) -> Settings {
        let mut guard = self
            .value
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        f(&mut guard);
        if let Err(e) = guard.save(&self.path) {
            log::error!("ayarlar kaydedilemedi: {e}");
        }
        guard.clone()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn temp_dir(name: &str) -> PathBuf {
        let dir =
            std::env::temp_dir().join(format!("clickfix-agent-test-{name}-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&dir);
        dir
    }

    #[test]
    fn defaults() {
        let s = Settings::default();
        assert!(s.close_to_tray);
        assert!(!s.autostart, "otomatik başlatma varsayılan kapalı");
        assert_eq!(s.shortcut, DEFAULT_SHORTCUT);
    }

    #[test]
    fn missing_or_broken_file_gives_defaults() {
        let dir = temp_dir("broken");
        assert_eq!(Settings::load(&dir.join(FILE_NAME)), Settings::default());
        std::fs::create_dir_all(&dir).expect("dizin");
        std::fs::write(dir.join(FILE_NAME), "{bozuk").expect("yaz");
        assert_eq!(Settings::load(&dir.join(FILE_NAME)), Settings::default());
        // Eksik alanlar varsayılanla dolar.
        std::fs::write(dir.join(FILE_NAME), r#"{"autostart":true}"#).expect("yaz");
        let s = Settings::load(&dir.join(FILE_NAME));
        assert!(s.autostart);
        assert!(s.close_to_tray);
        let _ = std::fs::remove_dir_all(&dir);
    }

    #[test]
    fn store_round_trip() {
        let dir = temp_dir("store");
        let store = SettingsStore::open(&dir);
        store.update(|s| {
            s.close_to_tray = false;
            s.shortcut = "Alt+Space".into();
        });
        let again = SettingsStore::open(&dir).get();
        assert!(!again.close_to_tray);
        assert_eq!(again.shortcut, "Alt+Space");
        let _ = std::fs::remove_dir_all(&dir);
    }
}
