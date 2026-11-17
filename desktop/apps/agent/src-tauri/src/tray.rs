//! Tepsi simgesi ve menüsü, uygulama menüsü, rozet.
//!
//! Tepsi menüsü: göster/gizle, durum (presence), "Kapatınca tepside kalsın",
//! "Oturum açılışında başlat", oturumu kapat, çık. Uygulama menüsü: Hakkında,
//! güncellemeleri denetle (CF-240 gelene kadar yer tutucu), günlükleri aç,
//! düzen ve pencere menüleri.

use tauri::image::Image;
use tauri::menu::{AboutMetadata, CheckMenuItem, Menu, MenuItem, PredefinedMenuItem, Submenu};
use tauri::tray::{MouseButton, MouseButtonState, TrayIcon, TrayIconBuilder, TrayIconEvent};
use tauri::{AppHandle, Manager, Wry};

use crate::settings::Settings;

pub const TRAY_ID: &str = "main";
const PRODUCT: &str = "ClickFix Masaüstü";

pub mod ids {
    pub const TOGGLE: &str = "tray-toggle";
    pub const PRESENCE_PREFIX: &str = "presence:";
    pub const CLOSE_TO_TRAY: &str = "tray-close-to-tray";
    pub const AUTOSTART: &str = "tray-autostart";
    pub const SIGN_OUT: &str = "tray-sign-out";
    pub const QUIT: &str = "app-quit";
    pub const CHECK_UPDATES: &str = "app-check-updates";
    pub const OPEN_LOGS: &str = "app-open-logs";
}

/// Presence modülünün durumları ve menü metinleri.
pub const PRESENCE: &[(&str, &str)] = &[
    ("online", "Çevrimiçi"),
    ("away", "Uzakta"),
    ("busy", "Meşgul"),
    ("offline", "Çevrimdışı"),
];

pub fn is_presence(status: &str) -> bool {
    PRESENCE.iter().any(|(s, _)| *s == status)
}

/// Menü öğesi kimliğinden presence durumu (`presence:busy` → `busy`).
pub fn presence_from_id(id: &str) -> Option<&'static str> {
    let status = id.strip_prefix(ids::PRESENCE_PREFIX)?;
    PRESENCE.iter().map(|(s, _)| *s).find(|s| *s == status)
}

/// Tepsi ipucu: okunmamış atanmış konuşma sayısıyla.
pub fn tooltip(unread: u32) -> String {
    match unread {
        0 => PRODUCT.to_owned(),
        n => format!("{PRODUCT} · {n} okunmamış konuşma"),
    }
}

/// Sonradan güncellenen menü öğeleri.
pub struct TrayHandles {
    pub tray: TrayIcon<Wry>,
    pub presence: Vec<(&'static str, CheckMenuItem<Wry>)>,
    pub close_to_tray: CheckMenuItem<Wry>,
    pub autostart: CheckMenuItem<Wry>,
}

impl TrayHandles {
    pub fn set_presence(&self, status: Option<&str>) {
        for (s, item) in &self.presence {
            let _ = item.set_checked(Some(*s) == status);
        }
    }
}

pub fn build_tray(app: &AppHandle, settings: &Settings) -> tauri::Result<TrayHandles> {
    let toggle = MenuItem::with_id(
        app,
        ids::TOGGLE,
        "ClickFix'i göster / gizle",
        true,
        None::<&str>,
    )?;
    let presence: Vec<(&'static str, CheckMenuItem<Wry>)> = PRESENCE
        .iter()
        .map(|(status, label)| {
            CheckMenuItem::with_id(
                app,
                format!("{}{status}", ids::PRESENCE_PREFIX),
                *label,
                true,
                false,
                None::<&str>,
            )
            .map(|item| (*status, item))
        })
        .collect::<tauri::Result<_>>()?;
    let presence_refs: Vec<&dyn tauri::menu::IsMenuItem<Wry>> = presence
        .iter()
        .map(|(_, i)| i as &dyn tauri::menu::IsMenuItem<Wry>)
        .collect();
    let status_menu = Submenu::with_items(app, "Durum", true, &presence_refs)?;
    let close_to_tray = CheckMenuItem::with_id(
        app,
        ids::CLOSE_TO_TRAY,
        "Kapatınca tepside kalsın",
        true,
        settings.close_to_tray,
        None::<&str>,
    )?;
    let autostart = CheckMenuItem::with_id(
        app,
        ids::AUTOSTART,
        "Oturum açılışında başlat",
        true,
        settings.autostart,
        None::<&str>,
    )?;
    let sign_out = MenuItem::with_id(app, ids::SIGN_OUT, "Oturumu kapat", true, None::<&str>)?;
    let quit = MenuItem::with_id(
        app,
        ids::QUIT,
        "ClickFix Masaüstü'nden çık",
        true,
        None::<&str>,
    )?;
    let menu = Menu::with_items(
        app,
        &[
            &toggle,
            &PredefinedMenuItem::separator(app)?,
            &status_menu,
            &PredefinedMenuItem::separator(app)?,
            &close_to_tray,
            &autostart,
            &PredefinedMenuItem::separator(app)?,
            &sign_out,
            &quit,
        ],
    )?;

    let mut builder = TrayIconBuilder::with_id(TRAY_ID)
        .tooltip(tooltip(0))
        .menu(&menu)
        .show_menu_on_left_click(false)
        .on_tray_icon_event(|tray, event| {
            if let TrayIconEvent::Click {
                button: MouseButton::Left,
                button_state: MouseButtonState::Up,
                ..
            } = event
            {
                crate::show_main(tray.app_handle());
            }
        });
    if let Some(icon) = app.default_window_icon() {
        builder = builder.icon(icon.clone());
    }
    let tray = builder.build(app)?;
    Ok(TrayHandles {
        tray,
        presence,
        close_to_tray,
        autostart,
    })
}

pub fn build_app_menu(app: &AppHandle) -> tauri::Result<Menu<Wry>> {
    let info = app.package_info();
    let about = PredefinedMenuItem::about(
        app,
        Some("ClickFix Masaüstü Hakkında"),
        Some(AboutMetadata {
            name: Some(PRODUCT.to_owned()),
            version: Some(info.version.to_string()),
            copyright: Some("© ClickSolve".to_owned()),
            website: Some("https://getclickfix.com".to_owned()),
            website_label: Some("getclickfix.com".to_owned()),
            ..Default::default()
        }),
    )?;
    let updates = MenuItem::with_id(
        app,
        ids::CHECK_UPDATES,
        "Güncellemeleri denetle…",
        true,
        None::<&str>,
    )?;
    let logs = MenuItem::with_id(app, ids::OPEN_LOGS, "Günlükleri aç", true, None::<&str>)?;
    let quit = MenuItem::with_id(app, ids::QUIT, "Çık", true, Some("CmdOrCtrl+Q"))?;
    let app_menu = Submenu::with_items(
        app,
        PRODUCT,
        true,
        &[
            &about,
            &PredefinedMenuItem::separator(app)?,
            &updates,
            &logs,
            &PredefinedMenuItem::separator(app)?,
            &PredefinedMenuItem::hide(app, Some("ClickFix'i gizle"))?,
            &PredefinedMenuItem::separator(app)?,
            &quit,
        ],
    )?;
    // Düzen menüsü macOS'ta webview'de kopyala/yapıştır kısayolları için gerekir.
    let edit = Submenu::with_items(
        app,
        "Düzen",
        true,
        &[
            &PredefinedMenuItem::undo(app, Some("Geri al"))?,
            &PredefinedMenuItem::redo(app, Some("Yinele"))?,
            &PredefinedMenuItem::separator(app)?,
            &PredefinedMenuItem::cut(app, Some("Kes"))?,
            &PredefinedMenuItem::copy(app, Some("Kopyala"))?,
            &PredefinedMenuItem::paste(app, Some("Yapıştır"))?,
            &PredefinedMenuItem::select_all(app, Some("Tümünü seç"))?,
        ],
    )?;
    let window = Submenu::with_items(
        app,
        "Pencere",
        true,
        &[
            &PredefinedMenuItem::minimize(app, Some("Küçült"))?,
            &PredefinedMenuItem::maximize(app, Some("Büyüt"))?,
            &PredefinedMenuItem::fullscreen(app, Some("Tam ekran"))?,
            &PredefinedMenuItem::separator(app)?,
            &PredefinedMenuItem::close_window(app, Some("Pencereyi kapat"))?,
        ],
    )?;
    Menu::with_items(app, &[&app_menu, &edit, &window])
}

/// Rozet: macOS dock ve Linux başlatıcı sayısı, Windows görev çubuğu
/// kaplaması; tepsi ipucu her platformda.
pub fn set_badge(app: &AppHandle, unread: u32) {
    if let Some(w) = app.get_webview_window(clickfix_shell::plugins::MAIN_WINDOW) {
        #[cfg(not(target_os = "windows"))]
        if let Err(e) = w.set_badge_count((unread > 0).then_some(i64::from(unread))) {
            log::debug!("rozet desteklenmiyor: {e}");
        }
        #[cfg(target_os = "windows")]
        {
            let icon = (unread > 0).then(overlay_dot);
            if let Err(e) = w.set_overlay_icon(icon) {
                log::debug!("görev çubuğu kaplaması ayarlanamadı: {e}");
            }
        }
        #[cfg(not(target_os = "windows"))]
        let _ = &w;
    }
    if let Some(h) = app.try_state::<TrayHandles>() {
        let _ = h.tray.set_tooltip(Some(tooltip(unread)));
    }
}

/// Windows görev çubuğu kaplaması: 16×16 kırmızı nokta.
#[cfg_attr(not(target_os = "windows"), allow(dead_code))]
pub fn overlay_dot() -> Image<'static> {
    const SIZE: u32 = 16;
    let mut rgba = Vec::with_capacity((SIZE * SIZE * 4) as usize);
    let c = (SIZE as f32 - 1.0) / 2.0;
    for y in 0..SIZE {
        for x in 0..SIZE {
            let d = ((x as f32 - c).powi(2) + (y as f32 - c).powi(2)).sqrt();
            let alpha = if d <= c - 0.5 {
                255
            } else if d <= c + 0.5 {
                128
            } else {
                0
            };
            rgba.extend_from_slice(&[0xE5, 0x48, 0x4D, alpha]);
        }
    }
    Image::new_owned(rgba, SIZE, SIZE)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn presence_ids() {
        assert_eq!(presence_from_id("presence:busy"), Some("busy"));
        assert_eq!(presence_from_id("presence:dans"), None);
        assert_eq!(presence_from_id("tray-quit"), None);
        assert!(is_presence("away"));
        assert!(!is_presence("Online"));
    }

    #[test]
    fn tooltip_counts() {
        assert_eq!(tooltip(0), "ClickFix Masaüstü");
        assert_eq!(tooltip(3), "ClickFix Masaüstü · 3 okunmamış konuşma");
    }

    #[test]
    fn overlay_is_square_rgba() {
        let img = overlay_dot();
        assert_eq!((img.width(), img.height()), (16, 16));
        assert_eq!(img.rgba().len(), 16 * 16 * 4);
        // Köşe saydam, merkez dolu.
        assert_eq!(img.rgba()[3], 0);
        let mid = ((8 * 16 + 8) * 4 + 3) as usize;
        assert_eq!(img.rgba()[mid], 255);
    }
}
