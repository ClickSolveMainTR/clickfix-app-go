//! `clickfix-support://join?t=<token>` derin bağlantısı (docs/design/masaustu.md
//! §5 Akış, 4. adım). Token tek kullanımlık katılma bağlantısıdır; arayüz onu
//! `session.resolve {token}` ile sidecar'a verir.

use url::Url;

pub const SCHEME: &str = "clickfix-support";

/// Token için kabul edilen uzunluk (backend 256 bitlik base64url üretir).
const MIN_TOKEN: usize = 16;
const MAX_TOKEN: usize = 256;

/// `url` geçerli bir katılma bağlantısıysa token'ı döndürür.
pub fn join_token(url: &Url) -> Option<String> {
    if url.scheme() != SCHEME || url.host_str() != Some("join") {
        return None;
    }
    if !url.path().trim_end_matches('/').is_empty() {
        return None;
    }
    let token = url
        .query_pairs()
        .find(|(k, _)| k == "t")
        .map(|(_, v)| v.into_owned())?;
    valid_token(&token).then_some(token)
}

fn valid_token(t: &str) -> bool {
    (MIN_TOKEN..=MAX_TOKEN).contains(&t.len())
        && t.bytes()
            .all(|b| b.is_ascii_alphanumeric() || b == b'-' || b == b'_')
}

#[cfg(test)]
mod tests {
    use super::*;

    fn parse(s: &str) -> Option<String> {
        join_token(&Url::parse(s).expect("adres"))
    }

    #[test]
    fn accepts_join_link() {
        assert_eq!(
            parse("clickfix-support://join?t=AbC_123-xyz456789"),
            Some("AbC_123-xyz456789".into())
        );
        assert_eq!(
            parse("clickfix-support://join/?t=AbC_123-xyz456789&x=1"),
            Some("AbC_123-xyz456789".into())
        );
    }

    #[test]
    fn rejects_other_links() {
        assert_eq!(parse("clickfix://join?t=AbC_123-xyz456789"), None);
        assert_eq!(parse("clickfix-support://open?t=AbC_123-xyz456789"), None);
        assert_eq!(
            parse("clickfix-support://join/extra?t=AbC_123-xyz456789"),
            None
        );
        assert_eq!(parse("clickfix-support://join"), None);
        assert_eq!(parse("clickfix-support://join?t=short"), None);
        assert_eq!(
            parse("clickfix-support://join?t=bad%20token%20value!!"),
            None
        );
        let long = "a".repeat(MAX_TOKEN + 1);
        assert_eq!(parse(&format!("clickfix-support://join?t={long}")), None);
    }
}
