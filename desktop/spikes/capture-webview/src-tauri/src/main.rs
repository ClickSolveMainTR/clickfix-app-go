// CF-235 aday A: yayıncı sayfasını (harness /publisher.html) Tauri penceresinde açar.
// Ekran yakalama webview'in getDisplayMedia'sı ile yapılır. macOS'ta WKWebView bunu
// varsayılan olarak açmıyor; CF235_PRIVATE=1 ile WebKit'in gizli (_) tercihleri denenir.
use tauri::Manager;

fn main() {
    tauri::Builder::default()
        .setup(|app| {
            #[cfg(target_os = "macos")]
            if std::env::var("CF235_PRIVATE").as_deref() != Ok("0") {
                let w = app.get_webview_window("main").expect("main penceresi yok");
                w.with_webview(|wv| unsafe { mac::enable(wv.inner().cast()) })?;
                let w2 = w.clone();
                std::thread::spawn(move || {
                    std::thread::sleep(std::time::Duration::from_millis(800));
                    let _ = w2.eval("location.reload()");
                });
            }
            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("tauri başlatılamadı");
}

#[cfg(target_os = "macos")]
mod mac {
    use objc2::runtime::{AnyObject, Bool, Sel};
    use objc2::{msg_send, sel};

    pub unsafe fn enable(wk: *mut AnyObject) {
        unsafe {
            let config: *mut AnyObject = msg_send![wk, configuration];
            let prefs: *mut AnyObject = msg_send![config, preferences];
            for (s, name) in [
                (sel!(_setScreenCaptureEnabled:), "_setScreenCaptureEnabled"),
                (sel!(_setMediaDevicesEnabled:), "_setMediaDevicesEnabled"),
                (sel!(_setMediaCaptureRequiresSecureConnection:), "_setMediaCaptureRequiresSecureConnection"),
                (sel!(_setGetUserMediaRequiresFocus:), "_setGetUserMediaRequiresFocus"),
            ] {
                let ok: Bool = msg_send![prefs, respondsToSelector: s];
                let val = !name.contains("Requires");
                if ok.as_bool() {
                    set(prefs, s, val);
                }
                eprintln!("cf235 {name}({val}) destek={}", ok.as_bool());
            }
        }
    }

    unsafe fn set(obj: *mut AnyObject, s: Sel, v: bool) {
        unsafe {
            let f: extern "C" fn(*mut AnyObject, Sel, Bool) =
                std::mem::transmute(objc2::ffi::objc_msgSend as *const ());
            f(obj, s, Bool::new(v));
        }
    }
}
