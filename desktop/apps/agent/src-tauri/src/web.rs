//! Rust → web görünümü yönü.
//!
//! - [`init_script`]: her sayfa yüklenmeden önce çalışır ve yalnız uygulama
//!   kökünde `window.__CLICKFIX_DESKTOP__` nesnesini kurar. Web köprüsü
//!   (`frontend/src/lib/desktop/bridge.ts`) masaüstünde olduğunu buradan
//!   anlar.
//! - [`event_script`]: tepsi, kısayol ve menüden gelen istekleri
//!   `clickfix:desktop` DOM olayı olarak gönderir.
//! - [`handoff_script`]: webview oturumunu kurar (CF-234, `POST
//!   /desktop/session`). Token URL'e yazılmaz; aynı kökenden gönderilen bir
//!   form gövdesinde gider.

use serde::Serialize;

/// Köprü sözleşmesinin sürümü. Yalnız geriye uyumlu ekleme yapılır; kırıcı
/// değişiklik yeni sürümle gelir (docs/webview-bridge.md kalıbı).
pub const BRIDGE_VERSION: u32 = 1;

/// Webview oturumunun kurulduğu Next rotası (CF-234).
pub const HANDOFF_PATH: &str = "/desktop/session";

/// Web uygulamasına giden olaylar (`detail.type`).
#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
#[serde(tag = "type", rename_all = "kebab-case")]
pub enum DesktopEvent {
    /// Genel kısayol: Cmd+K paletini aç.
    CommandPalette,
    /// Tepsi menüsünden durum seçildi.
    Presence { status: String },
    /// Tepsi menüsünden çıkış: web oturumunu da kapat.
    SignOut,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct DesktopInfo<'a> {
    bridge: u32,
    version: &'a str,
    os: &'a str,
}

/// `origin` (ör. `https://getclickfix.com`) dışındaki sayfalarda hiçbir şey
/// yapmayan başlangıç betiği.
pub fn init_script(origin: &str, version: &str, os: &str) -> String {
    let info = serde_json::to_string(&DesktopInfo {
        bridge: BRIDGE_VERSION,
        version,
        os,
    })
    .expect("sabit yapı");
    let origin = serde_json::to_string(origin).expect("metin");
    format!(
        "(function(){{if(window.location.origin!=={origin})return;\
         Object.defineProperty(window,'__CLICKFIX_DESKTOP__',\
         {{value:Object.freeze({info}),configurable:false,writable:false}});}})();"
    )
}

pub fn event_script(event: &DesktopEvent) -> String {
    let detail = serde_json::to_string(event).expect("olay");
    format!("window.dispatchEvent(new CustomEvent('clickfix:desktop',{{detail:{detail}}}));")
}

/// Uygulamanın erişim token'ını `POST /desktop/session`'a form ile gönderen
/// betik. `next` uygulama içi yoldur (`links::app_path` ile doğrulanmış).
pub fn handoff_script(access_token: &str, next: Option<&str>) -> String {
    let token = serde_json::to_string(access_token).expect("metin");
    let next = serde_json::to_string(&next).expect("metin");
    let action = serde_json::to_string(HANDOFF_PATH).expect("metin");
    format!(
        "(function(){{var f=document.createElement('form');f.method='POST';f.action={action};\
         function add(n,v){{var i=document.createElement('input');i.type='hidden';i.name=n;i.value=v;f.appendChild(i);}}\
         add('access_token',{token});var n={next};if(n)add('next',n);\
         (document.body||document.documentElement).appendChild(f);f.submit();}})();"
    )
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn events_are_tagged() {
        assert_eq!(
            event_script(&DesktopEvent::CommandPalette),
            "window.dispatchEvent(new CustomEvent('clickfix:desktop',{detail:{\"type\":\"command-palette\"}}));"
        );
        assert!(
            event_script(&DesktopEvent::Presence {
                status: "busy".into()
            })
            .contains(r#"{"type":"presence","status":"busy"}"#)
        );
        assert!(event_script(&DesktopEvent::SignOut).contains(r#"{"type":"sign-out"}"#));
    }

    #[test]
    fn init_script_is_origin_bound() {
        let s = init_script("https://getclickfix.com", "0.1.0", "macos");
        assert!(s.contains(r#"window.location.origin!=="https://getclickfix.com""#));
        assert!(s.contains(r#"{"bridge":1,"version":"0.1.0","os":"macos"}"#));
    }

    #[test]
    fn handoff_escapes_values_and_posts_to_session_route() {
        let s = handoff_script("a.b.c", Some("/inbox?c=1\"</script>"));
        assert!(s.contains(r#"f.action="/desktop/session""#));
        assert!(s.contains(r#"add('access_token',"a.b.c")"#));
        assert!(s.contains(r#"var n="/inbox?c=1\"</script>""#));
        assert!(handoff_script("a.b.c", None).contains("var n=null"));
    }
}
