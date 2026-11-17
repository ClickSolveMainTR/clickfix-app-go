//! `clickfix://` derin bağlantıları ve uygulama içi yolların doğrulanması.
//!
//! Desteklenen bağlantılar (docs/design/masaustu.md §4, §6):
//!
//! - `clickfix://auth/callback?code=...&state=...`: sistem tarayıcısındaki
//!   girişten dönüş (CF-234).
//! - `clickfix://conversation/<uuid>`: konuşmayı açar (`/inbox?c=<uuid>`).
//! - `clickfix://open?path=/...`: uygulama kökü altındaki bir sayfayı açar
//!   (e-postadaki "Masaüstünde aç" bağlantısı, `/desktop/open` sayfası).
//! - `clickfix://` (ya da tanınmayan biçim): yalnız pencereyi öne getirir.

use url::Url;

pub const SCHEME: &str = "clickfix";

/// Uygulama yolu için üst sınır (URL'nin tamamı tarayıcılarda ~2 KB'ta
/// güvenle taşınır).
const MAX_PATH: usize = 2048;

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum DeepLink {
    AuthCallback { code: String, state: String },
    Open { path: String },
    Focus,
}

/// `url` bir `clickfix://` bağlantısıysa ne istendiğini döndürür; başka şema
/// ise `None`.
pub fn parse(url: &Url) -> Option<DeepLink> {
    if url.scheme() != SCHEME {
        return None;
    }
    let host = url.host_str().unwrap_or_default();
    let path = url.path().trim_end_matches('/');
    let link = match (host, path) {
        ("auth", "/callback") => {
            let code = query_value(url, "code");
            let state = query_value(url, "state");
            match (code, state) {
                (Some(code), Some(state)) if valid_code(&code) && valid_state(&state) => {
                    DeepLink::AuthCallback { code, state }
                }
                _ => DeepLink::Focus,
            }
        }
        ("conversation", p) => match p.strip_prefix('/') {
            Some(id) if is_uuid(id) => DeepLink::Open {
                path: format!("/inbox?c={}", id.to_ascii_lowercase()),
            },
            _ => DeepLink::Focus,
        },
        ("open", "") => match query_value(url, "path").and_then(|p| app_path(&p)) {
            Some(path) => DeepLink::Open { path },
            None => DeepLink::Focus,
        },
        _ => DeepLink::Focus,
    };
    Some(link)
}

fn query_value(url: &Url, key: &str) -> Option<String> {
    url.query_pairs()
        .find(|(k, _)| k == key)
        .map(|(_, v)| v.into_owned())
}

/// Uygulama kökü altındaki göreli yol (`/inbox?c=...`). `//host`, ters eğik
/// çizgi, denetim karakteri ve aşırı uzun değerler reddedilir; böylece yol
/// köke eklendiğinde başka bir kökene gidemez.
pub fn app_path(raw: &str) -> Option<String> {
    let p = raw.trim();
    if p.is_empty()
        || p.len() > MAX_PATH
        || !p.starts_with('/')
        || p.starts_with("//")
        || p.contains('\\')
        || p.chars().any(char::is_control)
    {
        return None;
    }
    Some(p.to_owned())
}

/// `path`'i uygulama köküne ekler; sonuç kökle aynı kökende değilse `None`.
pub fn resolve(root: &Url, path: &str) -> Option<Url> {
    let path = app_path(path)?;
    let url = root.join(&path).ok()?;
    (url.origin() == root.origin()).then_some(url)
}

/// API'nin tek kullanımlık kodu: 43 karakter base64url (CF-234).
pub fn valid_code(v: &str) -> bool {
    v.len() == 43 && v.bytes().all(is_base64url)
}

/// `state`: 16–128 karakter, RFC 3986 "unreserved" (CF-234 ile aynı kural).
pub fn valid_state(v: &str) -> bool {
    (16..=128).contains(&v.len())
        && v.bytes()
            .all(|b| b.is_ascii_alphanumeric() || matches!(b, b'.' | b'_' | b'~' | b'-'))
}

fn is_base64url(b: u8) -> bool {
    b.is_ascii_alphanumeric() || b == b'-' || b == b'_'
}

fn is_uuid(v: &str) -> bool {
    let b = v.as_bytes();
    b.len() == 36
        && b.iter().enumerate().all(|(i, c)| match i {
            8 | 13 | 18 | 23 => *c == b'-',
            _ => c.is_ascii_hexdigit(),
        })
}

/// Sistem tarayıcısında açılabilecek dış bağlantı: yalnız `https`, `http` ve
/// `mailto`. Diğer şemalar (`file:`, `javascript:`, özel şemalar) reddedilir.
pub fn external_url(raw: &str) -> Option<Url> {
    let url = Url::parse(raw.trim()).ok()?;
    match url.scheme() {
        "https" | "http" if url.host_str().is_some() => Some(url),
        "mailto" => Some(url),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn link(s: &str) -> Option<DeepLink> {
        parse(&Url::parse(s).expect("url"))
    }

    const CODE: &str = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA";

    #[test]
    fn auth_callback() {
        assert_eq!(
            link(&format!(
                "clickfix://auth/callback?code={CODE}&state=state-0123456789abcdef"
            )),
            Some(DeepLink::AuthCallback {
                code: CODE.into(),
                state: "state-0123456789abcdef".into()
            })
        );
        // Bozuk kod ya da state: yalnız öne getir.
        assert_eq!(
            link("clickfix://auth/callback?code=kisa&state=state-0123456789abcdef"),
            Some(DeepLink::Focus)
        );
        assert_eq!(
            link(&format!("clickfix://auth/callback?code={CODE}&state=kisa")),
            Some(DeepLink::Focus)
        );
    }

    #[test]
    fn conversation() {
        assert_eq!(
            link("clickfix://conversation/0B9C8A1E-1111-4222-8333-444455556666"),
            Some(DeepLink::Open {
                path: "/inbox?c=0b9c8a1e-1111-4222-8333-444455556666".into()
            })
        );
        assert_eq!(
            link("clickfix://conversation/../../etc"),
            Some(DeepLink::Focus)
        );
        assert_eq!(link("clickfix://conversation/"), Some(DeepLink::Focus));
    }

    #[test]
    fn open_path() {
        assert_eq!(
            link("clickfix://open?path=%2Fbooking%3Fa%3D1"),
            Some(DeepLink::Open {
                path: "/booking?a=1".into()
            })
        );
        for bad in [
            "clickfix://open?path=%2F%2Fevil.test",
            "clickfix://open?path=https%3A%2F%2Fevil.test",
            "clickfix://open?path=%2F%5Cevil.test",
            "clickfix://open",
        ] {
            assert_eq!(link(bad), Some(DeepLink::Focus), "{bad}");
        }
    }

    #[test]
    fn other_schemes_ignored() {
        assert_eq!(link("https://getclickfix.com/inbox"), None);
        assert_eq!(link("clickfix://"), Some(DeepLink::Focus));
    }

    #[test]
    fn resolve_stays_on_root() {
        let root = Url::parse("https://getclickfix.com/").expect("url");
        assert_eq!(
            resolve(&root, "/inbox?c=1").map(String::from),
            Some("https://getclickfix.com/inbox?c=1".into())
        );
        assert_eq!(resolve(&root, "//evil.test/x"), None);
        assert_eq!(resolve(&root, "/\\evil.test"), None);
        assert_eq!(resolve(&root, "inbox"), None);
        assert_eq!(resolve(&root, "/a\nb"), None);
        assert_eq!(resolve(&root, &format!("/{}", "a".repeat(MAX_PATH))), None);
    }

    #[test]
    fn external_urls() {
        assert!(external_url("https://example.com/a").is_some());
        assert!(external_url("mailto:destek@example.com").is_some());
        assert!(external_url("javascript:alert(1)").is_none());
        assert!(external_url("file:///etc/passwd").is_none());
        assert!(external_url("clickfix://open").is_none());
        assert!(external_url("bozuk").is_none());
    }
}
