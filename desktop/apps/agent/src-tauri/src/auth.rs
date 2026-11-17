//! Sistem tarayıcısıyla giriş (CF-234 akışının uygulama tarafı,
//! docs/design/masaustu.md §6).
//!
//! 1. [`Pkce::generate`] `state` ve `code_verifier` üretir; tarayıcıda
//!    [`login_url`] açılır.
//! 2. Tarayıcı `clickfix://auth/callback?code=...&state=...` ile döner;
//!    [`Api::exchange`] kodu ve verifier'ı `POST /v1/auth/desktop/exchange`
//!    ile takas eder (`/api/mobile/v1` üzerinden).
//! 3. Refresh token işletim sisteminin anahtar zincirinde ([`TokenStore`])
//!    durur; erişim token'ı yalnız bellekte tutulur ([`Session`]).
//!
//! Token'lar, kodlar ve verifier hiçbir zaman günlüğe yazılmaz.

use std::time::{Duration, Instant};

use base64::Engine;
use base64::engine::general_purpose::URL_SAFE_NO_PAD;
use serde::Deserialize;
use sha2::{Digest, Sha256};
use url::Url;

/// Bekleyen bir girişin geçerlilik süresi. API kodu 60 sn tutar; kullanıcının
/// tarayıcıda giriş yapması (ikinci adım dahil) daha uzun sürebilir.
pub const LOGIN_TTL: Duration = Duration::from_secs(15 * 60);

/// Erişim token'ı bu kadar süre kala yenilenir.
const ACCESS_MARGIN: Duration = Duration::from_secs(60);

const HTTP_TIMEOUT: Duration = Duration::from_secs(20);

#[derive(Debug, thiserror::Error)]
pub enum AuthError {
    /// Sunucu isteği reddetti (geçersiz kod, iptal edilmiş oturum). Saklanan
    /// refresh token artık kullanılamaz.
    #[error("oturum reddedildi ({0})")]
    Rejected(u16),
    #[error("sunucuya ulaşılamadı: {0}")]
    Network(String),
    #[error("beklenmeyen yanıt: {0}")]
    Unexpected(String),
    #[error("anahtar zinciri: {0}")]
    Keychain(String),
    #[error("rastgele sayı üretilemedi: {0}")]
    Random(String),
}

/// PKCE (RFC 7636, S256) ve `state`.
#[derive(Clone)]
pub struct Pkce {
    pub state: String,
    pub verifier: String,
    pub challenge: String,
}

impl std::fmt::Debug for Pkce {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("Pkce").finish_non_exhaustive()
    }
}

impl Pkce {
    pub fn generate() -> Result<Self, AuthError> {
        let mut verifier = [0u8; 32];
        let mut state = [0u8; 24];
        getrandom::fill(&mut verifier).map_err(|e| AuthError::Random(e.to_string()))?;
        getrandom::fill(&mut state).map_err(|e| AuthError::Random(e.to_string()))?;
        let verifier = URL_SAFE_NO_PAD.encode(verifier);
        Ok(Self {
            state: URL_SAFE_NO_PAD.encode(state),
            challenge: challenge(&verifier),
            verifier,
        })
    }
}

/// `BASE64URL(SHA256(verifier))`, dolgusuz.
pub fn challenge(verifier: &str) -> String {
    URL_SAFE_NO_PAD.encode(Sha256::digest(verifier.as_bytes()))
}

/// Sistem tarayıcısında açılan giriş sayfası: `<kök>/desktop/login?state&challenge`.
pub fn login_url(root: &Url, pkce: &Pkce) -> Url {
    let mut url = root.join("desktop/login").expect("sabit yol");
    url.query_pairs_mut()
        .append_pair("state", &pkce.state)
        .append_pair("challenge", &pkce.challenge);
    url
}

/// Tarayıcıda başlatılmış, dönüşü beklenen giriş.
#[derive(Debug, Clone)]
pub struct PendingLogin {
    pub pkce: Pkce,
    pub started: Instant,
}

impl PendingLogin {
    pub fn new(pkce: Pkce) -> Self {
        Self {
            pkce,
            started: Instant::now(),
        }
    }

    /// Dönüşteki `state` bu girişe ait ve süresi dolmamış mı?
    pub fn matches(&self, state: &str, now: Instant) -> bool {
        now.duration_since(self.started) <= LOGIN_TTL && constant_eq(&self.pkce.state, state)
    }
}

fn constant_eq(a: &str, b: &str) -> bool {
    a.len() == b.len()
        && a.bytes()
            .zip(b.bytes())
            .fold(0u8, |acc, (x, y)| acc | (x ^ y))
            == 0
}

/// API'nin döndürdüğü token çifti.
#[derive(Clone, Deserialize)]
pub struct Tokens {
    pub access_token: String,
    pub refresh_token: String,
    #[serde(default)]
    pub expires_in: u64,
}

impl std::fmt::Debug for Tokens {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("Tokens")
            .field("expires_in", &self.expires_in)
            .finish_non_exhaustive()
    }
}

#[derive(Deserialize)]
struct Envelope<T> {
    data: Option<T>,
}

/// Masaüstü platformu, API'nin `client_os` değeri.
pub fn client_os() -> &'static str {
    match std::env::consts::OS {
        "macos" => "macos",
        "windows" => "windows",
        _ => "linux",
    }
}

/// Cihaz listesinde görünen bilgisayar adı (≤ 64 karakter, denetim
/// karakteri yok). Bulunamazsa `None`.
pub fn device_name() -> Option<String> {
    let raw = if cfg!(target_os = "windows") {
        std::env::var("COMPUTERNAME").ok()
    } else if cfg!(target_os = "macos") {
        std::process::Command::new("scutil")
            .args(["--get", "ComputerName"])
            .output()
            .ok()
            .filter(|o| o.status.success())
            .and_then(|o| String::from_utf8(o.stdout).ok())
    } else {
        std::fs::read_to_string("/etc/hostname")
            .ok()
            .or_else(|| std::env::var("HOSTNAME").ok())
    }?;
    clean_device_name(&raw)
}

pub fn clean_device_name(raw: &str) -> Option<String> {
    let name: String = raw
        .trim()
        .chars()
        .filter(|c| !c.is_control())
        .take(64)
        .collect();
    let name = name.trim().to_owned();
    (!name.is_empty()).then_some(name)
}

/// ClickFix API istemcisi (yerel istemci yolu `/api/mobile/v1`).
#[derive(Clone)]
pub struct Api {
    http: reqwest::Client,
    base: Url,
}

impl Api {
    pub fn new(root: &Url, user_agent: &str) -> Result<Self, AuthError> {
        let http = reqwest::Client::builder()
            .timeout(HTTP_TIMEOUT)
            .user_agent(user_agent)
            .https_only(root.scheme() == "https")
            .build()
            .map_err(|e| AuthError::Unexpected(e.to_string()))?;
        Ok(Self {
            http,
            base: api_base(root),
        })
    }

    pub async fn exchange(&self, code: &str, pkce: &Pkce) -> Result<Tokens, AuthError> {
        let mut body = serde_json::json!({
            "code": code,
            "code_verifier": pkce.verifier,
            "state": pkce.state,
            "client_os": client_os(),
        });
        if let Some(name) = device_name() {
            body["device_name"] = name.into();
        }
        self.post_tokens("auth/desktop/exchange", &body, None).await
    }

    pub async fn refresh(&self, refresh_token: &str) -> Result<Tokens, AuthError> {
        let body = serde_json::json!({ "refresh_token": refresh_token });
        self.post_tokens("auth/refresh", &body, None).await
    }

    /// Uygulamanın oturumunu sunucuda kapatır (refresh token iptal edilir).
    pub async fn logout(&self, access_token: &str, refresh_token: &str) -> Result<(), AuthError> {
        let url = self.base.join("auth/logout").expect("sabit yol");
        let res = self
            .http
            .post(url)
            .bearer_auth(access_token)
            .json(&serde_json::json!({ "refresh_token": refresh_token }))
            .send()
            .await
            .map_err(|e| AuthError::Network(e.without_url().to_string()))?;
        let status = res.status();
        if status.is_success() {
            Ok(())
        } else if status.is_client_error() {
            Err(AuthError::Rejected(status.as_u16()))
        } else {
            Err(AuthError::Unexpected(format!("HTTP {status}")))
        }
    }

    async fn post_tokens(
        &self,
        path: &str,
        body: &serde_json::Value,
        bearer: Option<&str>,
    ) -> Result<Tokens, AuthError> {
        let url = self.base.join(path).expect("sabit yol");
        let mut req = self.http.post(url).json(body);
        if let Some(t) = bearer {
            req = req.bearer_auth(t);
        }
        let res = req
            .send()
            .await
            .map_err(|e| AuthError::Network(e.without_url().to_string()))?;
        let status = res.status();
        if status.is_client_error() {
            return Err(AuthError::Rejected(status.as_u16()));
        }
        if !status.is_success() {
            return Err(AuthError::Unexpected(format!("HTTP {status}")));
        }
        let env: Envelope<Tokens> = res
            .json()
            .await
            .map_err(|e| AuthError::Unexpected(e.without_url().to_string()))?;
        env.data
            .filter(|t| !t.access_token.is_empty() && !t.refresh_token.is_empty())
            .ok_or_else(|| AuthError::Unexpected("token yok".into()))
    }
}

/// `<kök>/api/mobile/v1/`: Next.js yerel istemci geçidi (Bearer aynen geçer,
/// panel BFF'i masaüstü takasını reddeder).
pub fn api_base(root: &Url) -> Url {
    root.join("api/mobile/v1/").expect("sabit yol")
}

/// Refresh token'ın kalıcı deposu.
pub trait TokenStore: Send + Sync {
    fn load(&self) -> Result<Option<String>, AuthError>;
    fn save(&self, token: &str) -> Result<(), AuthError>;
    fn clear(&self) -> Result<(), AuthError>;
}

/// İşletim sisteminin anahtar zinciri: macOS Keychain, Windows Credential
/// Manager, Linux Secret Service. Kayıt uygulama köküne göre ayrılır; yerel
/// geliştirme ve hazırlık ortamı üretim oturumunu ezmez.
pub struct Keychain {
    service: String,
    account: String,
}

impl Keychain {
    pub fn new(identifier: &str, root: &Url) -> Self {
        Self {
            service: identifier.to_owned(),
            account: format!("refresh-token {}", root.origin().ascii_serialization()),
        }
    }

    fn entry(&self) -> Result<keyring::Entry, AuthError> {
        keyring::Entry::new(&self.service, &self.account)
            .map_err(|e| AuthError::Keychain(e.to_string()))
    }
}

impl TokenStore for Keychain {
    fn load(&self) -> Result<Option<String>, AuthError> {
        match self.entry()?.get_password() {
            Ok(t) if !t.is_empty() => Ok(Some(t)),
            Ok(_) | Err(keyring::Error::NoEntry) => Ok(None),
            Err(e) => Err(AuthError::Keychain(e.to_string())),
        }
    }

    fn save(&self, token: &str) -> Result<(), AuthError> {
        self.entry()?
            .set_password(token)
            .map_err(|e| AuthError::Keychain(e.to_string()))
    }

    fn clear(&self) -> Result<(), AuthError> {
        match self.entry()?.delete_credential() {
            Ok(()) | Err(keyring::Error::NoEntry) => Ok(()),
            Err(e) => Err(AuthError::Keychain(e.to_string())),
        }
    }
}

/// Bellekteki erişim token'ı.
struct Access {
    token: String,
    expires: Instant,
}

/// Uygulamanın oturumu. Refresh rotasyonu tek kullanımlık olduğundan
/// yenileme bir kilit altında, sırayla yapılır.
pub struct Session {
    api: Api,
    store: Box<dyn TokenStore>,
    access: tokio::sync::Mutex<Option<Access>>,
}

impl Session {
    pub fn new(api: Api, store: Box<dyn TokenStore>) -> Self {
        Self {
            api,
            store,
            access: tokio::sync::Mutex::new(None),
        }
    }

    pub fn api(&self) -> &Api {
        &self.api
    }

    /// Takas ya da yenilemeden gelen çifti kaydeder.
    async fn remember(
        &self,
        slot: &mut Option<Access>,
        tokens: Tokens,
    ) -> Result<String, AuthError> {
        self.store.save(&tokens.refresh_token)?;
        let lifetime = Duration::from_secs(tokens.expires_in.max(60));
        *slot = Some(Access {
            token: tokens.access_token.clone(),
            expires: Instant::now() + lifetime,
        });
        Ok(tokens.access_token)
    }

    /// Takas sonucu oturumu açar.
    pub async fn sign_in(&self, tokens: Tokens) -> Result<String, AuthError> {
        let mut slot = self.access.lock().await;
        self.remember(&mut slot, tokens).await
    }

    /// Geçerli erişim token'ı; gerekirse refresh token ile yeniler. Oturum
    /// yoksa ya da sunucu reddederse `Ok(None)` (saklanan token silinir).
    pub async fn access_token(&self) -> Result<Option<String>, AuthError> {
        let mut slot = self.access.lock().await;
        if let Some(a) = slot.as_ref()
            && a.expires > Instant::now() + ACCESS_MARGIN
        {
            return Ok(Some(a.token.clone()));
        }
        // Okunamayan anahtar zinciri (ör. Linux'ta Secret Service yok) oturum
        // yok sayılır; kullanıcı yeniden giriş yapınca kayıt hatası gösterilir.
        let stored = self.store.load().unwrap_or_else(|e| {
            log::warn!("anahtar zinciri okunamadı: {e}");
            None
        });
        let Some(refresh) = stored else {
            *slot = None;
            return Ok(None);
        };
        match self.api.refresh(&refresh).await {
            Ok(tokens) => self.remember(&mut slot, tokens).await.map(Some),
            Err(AuthError::Rejected(code)) => {
                log::warn!("saklanan oturum reddedildi (HTTP {code}), çıkış yapıldı");
                *slot = None;
                self.store.clear()?;
                Ok(None)
            }
            Err(e) => Err(e),
        }
    }

    /// Oturumu kapatır: sunucuda iptal etmeyi dener, her durumda yerel
    /// kaydı siler.
    pub async fn sign_out(&self) -> Result<(), AuthError> {
        let mut slot = self.access.lock().await;
        let refresh = self.store.load().unwrap_or(None);
        let access = slot.take();
        if let Some(refresh) = refresh {
            let access = match access {
                Some(a) if a.expires > Instant::now() + ACCESS_MARGIN => Some((a.token, refresh)),
                _ => match self.api.refresh(&refresh).await {
                    Ok(t) => Some((t.access_token, t.refresh_token)),
                    Err(e) => {
                        log::warn!("çıkışta yenileme başarısız: {e}");
                        None
                    }
                },
            };
            if let Some((access, refresh)) = access
                && let Err(e) = self.api.logout(&access, &refresh).await
            {
                log::warn!("sunucuda çıkış başarısız: {e}");
            }
        }
        self.store.clear()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Mutex;

    #[test]
    fn rfc7636_challenge() {
        // RFC 7636 Ek B.
        assert_eq!(
            challenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"),
            "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
        );
    }

    #[test]
    fn generated_pkce_matches_api_rules() {
        let a = Pkce::generate().expect("pkce");
        let b = Pkce::generate().expect("pkce");
        assert_eq!(a.verifier.len(), 43);
        assert_eq!(a.challenge, challenge(&a.verifier));
        assert!(crate::links::valid_state(&a.state));
        assert_ne!(a.state, b.state);
        assert_ne!(a.verifier, b.verifier);
        // Debug çıktısı sır taşımaz.
        assert!(!format!("{a:?}").contains(&a.verifier));
    }

    #[test]
    fn login_url_carries_state_and_challenge_only() {
        let root = Url::parse("https://getclickfix.com/").expect("url");
        let p = Pkce::generate().expect("pkce");
        let url = login_url(&root, &p);
        assert_eq!(url.path(), "/desktop/login");
        let q: Vec<(String, String)> = url.query_pairs().into_owned().collect();
        assert_eq!(
            q,
            vec![
                ("state".into(), p.state.clone()),
                ("challenge".into(), p.challenge.clone())
            ]
        );
        assert!(!url.as_str().contains(&p.verifier));
    }

    #[test]
    fn pending_login_state_and_ttl() {
        let p = PendingLogin::new(Pkce::generate().expect("pkce"));
        let now = p.started;
        assert!(p.matches(&p.pkce.state.clone(), now));
        assert!(!p.matches("state-0123456789abcdef", now));
        assert!(!p.matches(
            &p.pkce.state.clone(),
            now + LOGIN_TTL + Duration::from_secs(1)
        ));
    }

    #[test]
    fn api_base_is_native_client_path() {
        let root = Url::parse("http://localhost:3000/").expect("url");
        assert_eq!(
            api_base(&root)
                .join("auth/desktop/exchange")
                .expect("url")
                .as_str(),
            "http://localhost:3000/api/mobile/v1/auth/desktop/exchange"
        );
    }

    #[test]
    fn device_name_is_cleaned() {
        assert_eq!(
            clean_device_name("  Ayşe'nin MacBook\u{7}\n"),
            Some("Ayşe'nin MacBook".into())
        );
        assert_eq!(clean_device_name(" \n"), None);
        assert_eq!(
            clean_device_name(&"x".repeat(100)).map(|s| s.len()),
            Some(64)
        );
    }

    #[test]
    fn keychain_account_per_origin() {
        let a = Keychain::new(
            "com.getclickfix.desktop",
            &Url::parse("https://getclickfix.com/").expect("url"),
        );
        let b = Keychain::new(
            "com.getclickfix.desktop",
            &Url::parse("http://localhost:3000/").expect("url"),
        );
        assert_eq!(a.account, "refresh-token https://getclickfix.com");
        assert_ne!(a.account, b.account);
    }

    #[test]
    fn tokens_debug_hides_secrets() {
        let t = Tokens {
            access_token: "gizli-erisim".into(),
            refresh_token: "gizli-yenileme".into(),
            expires_in: 900,
        };
        let s = format!("{t:?}");
        assert!(!s.contains("gizli"));
    }

    /// Bellek içi depo; ağ gerektirmeyen oturum davranışları için.
    #[derive(Default)]
    struct MemStore(Mutex<Option<String>>);

    impl TokenStore for MemStore {
        fn load(&self) -> Result<Option<String>, AuthError> {
            Ok(self.0.lock().expect("kilit").clone())
        }
        fn save(&self, token: &str) -> Result<(), AuthError> {
            *self.0.lock().expect("kilit") = Some(token.into());
            Ok(())
        }
        fn clear(&self) -> Result<(), AuthError> {
            *self.0.lock().expect("kilit") = None;
            Ok(())
        }
    }

    #[tokio::test]
    async fn session_keeps_refresh_in_store_and_access_in_memory() {
        let root = Url::parse("http://127.0.0.1:9/").expect("url");
        let s = Session::new(
            Api::new(&root, "test").expect("api"),
            Box::new(MemStore::default()),
        );
        assert_eq!(s.access_token().await.expect("boş oturum"), None);

        let access = s
            .sign_in(Tokens {
                access_token: "a1".into(),
                refresh_token: "r1".into(),
                expires_in: 900,
            })
            .await
            .expect("giriş");
        assert_eq!(access, "a1");
        // Süresi dolmamış erişim token'ı ağa çıkmadan döner.
        assert_eq!(s.access_token().await.expect("token"), Some("a1".into()));
    }
}
