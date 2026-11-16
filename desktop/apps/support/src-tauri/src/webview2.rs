//! Windows: WebView2 Runtime yoksa arayüz hiç açılamaz. Kurulumsuz tek exe
//! (portable) WebView2'yi kendisi kuramadığından, pencere açmadan önce yerel
//! bir iletişim kutusuyla durumu anlatır ve Microsoft'un indirme sayfasını
//! açmayı önerir. Metin iki dillidir (sistem dili webview olmadan okunmaz).

use rfd::{MessageButtons, MessageDialog, MessageDialogResult, MessageLevel};

/// Microsoft'un Evergreen WebView2 Runtime önyükleyicisi.
pub const DOWNLOAD_URL: &str = "https://go.microsoft.com/fwlink/p/?LinkId=2124703";

const DOWNLOAD: &str = "İndir / Download";
const CLOSE: &str = "Kapat / Close";

/// WebView2 kurulu değilse açıklar ve `true` döner (uygulama kapanmalı).
pub fn explain_if_missing() -> bool {
    if tauri::webview_version().is_ok() {
        return false;
    }
    let result = MessageDialog::new()
        .set_level(MessageLevel::Warning)
        .set_title("ClickFix Destek")
        .set_description(
            "ClickFix Destek, Windows'un WebView2 bileşenine ihtiyaç duyar ve bu bilgisayarda \
             bulunamadı. Microsoft'tan ücretsiz indirip kurun, ardından ClickFix Destek'i \
             yeniden açın.\n\n\
             ClickFix Support needs the Microsoft WebView2 Runtime, which is missing on this \
             computer. Download and install it for free from Microsoft, then open ClickFix \
             Support again.",
        )
        .set_buttons(MessageButtons::OkCancelCustom(
            DOWNLOAD.into(),
            CLOSE.into(),
        ))
        .show();
    let download = match result {
        MessageDialogResult::Custom(label) => label == DOWNLOAD,
        MessageDialogResult::Ok | MessageDialogResult::Yes => true,
        _ => false,
    };
    if download && let Err(e) = tauri_plugin_opener::open_url(DOWNLOAD_URL, None::<&str>) {
        log::error!("WebView2 indirme sayfası açılamadı: {e}");
    }
    true
}
