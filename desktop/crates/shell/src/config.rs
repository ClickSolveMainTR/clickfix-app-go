//! Ortam değişkenlerinden okunan kabuk yapılandırması.

use url::Url;

/// Temsilci uygulamasının yüklediği varsayılan uzak kök.
pub const DEFAULT_APP_URL: &str = "https://getclickfix.com";

/// Uzak kökü değiştiren ortam değişkeni.
pub const APP_URL_ENV: &str = "CLICKFIX_APP_URL";

#[derive(Debug, thiserror::Error, PartialEq, Eq)]
pub enum ConfigError {
    #[error("{APP_URL_ENV} geçerli bir URL değil: {0}")]
    InvalidUrl(String),
    #[error("{APP_URL_ENV} https olmalı (yalnız localhost için http kabul edilir): {0}")]
    InsecureUrl(String),
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct AppConfig {
    /// Uygulama kökü; yol, sorgu ve parça içermez.
    pub app_url: Url,
}

impl AppConfig {
    /// Süreç ortamından okur; değişken yoksa ya da boşsa varsayılanı kullanır.
    pub fn from_env() -> Result<Self, ConfigError> {
        Self::from_value(std::env::var(APP_URL_ENV).ok().as_deref())
    }

    pub fn from_value(value: Option<&str>) -> Result<Self, ConfigError> {
        let raw = value
            .map(str::trim)
            .filter(|v| !v.is_empty())
            .unwrap_or(DEFAULT_APP_URL);
        let mut url = Url::parse(raw).map_err(|_| ConfigError::InvalidUrl(raw.to_owned()))?;
        let local = matches!(url.host_str(), Some("localhost" | "127.0.0.1" | "[::1]"));
        match url.scheme() {
            "https" => {}
            "http" if local => {}
            "http" => return Err(ConfigError::InsecureUrl(raw.to_owned())),
            _ => return Err(ConfigError::InvalidUrl(raw.to_owned())),
        }
        if url.host_str().is_none() {
            return Err(ConfigError::InvalidUrl(raw.to_owned()));
        }
        url.set_path("/");
        url.set_query(None);
        url.set_fragment(None);
        Ok(Self { app_url: url })
    }

    /// Tauri `remote.urls` için kökün altındaki her sayfayı kapsayan desen.
    pub fn remote_pattern(&self) -> String {
        format!("{}*", self.app_url)
    }

    /// `url` uygulama köküyle aynı kökenden mi (şema, ana makine, port)?
    pub fn is_app_origin(&self, url: &Url) -> bool {
        url.origin() == self.app_url.origin()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn defaults_when_unset_or_blank() {
        for v in [None, Some(""), Some("   ")] {
            let c = AppConfig::from_value(v).expect("varsayılan geçerli");
            assert_eq!(c.app_url.as_str(), "https://getclickfix.com/");
        }
    }

    #[test]
    fn normalises_to_origin_root() {
        let c = AppConfig::from_value(Some("https://staging.getclickfix.com/tr/inbox?x=1#y"))
            .expect("geçerli");
        assert_eq!(c.app_url.as_str(), "https://staging.getclickfix.com/");
        assert_eq!(c.remote_pattern(), "https://staging.getclickfix.com/*");
    }

    #[test]
    fn allows_http_only_on_localhost() {
        assert!(AppConfig::from_value(Some("http://localhost:3000")).is_ok());
        assert!(AppConfig::from_value(Some("http://127.0.0.1:3000")).is_ok());
        assert_eq!(
            AppConfig::from_value(Some("http://getclickfix.com")),
            Err(ConfigError::InsecureUrl("http://getclickfix.com".into()))
        );
        assert!(matches!(
            AppConfig::from_value(Some("file:///etc/passwd")),
            Err(ConfigError::InvalidUrl(_))
        ));
        assert!(matches!(
            AppConfig::from_value(Some("bozuk")),
            Err(ConfigError::InvalidUrl(_))
        ));
    }

    #[test]
    fn origin_check() {
        let c = AppConfig::from_value(None).expect("geçerli");
        let ok = Url::parse("https://getclickfix.com/inbox/1").expect("url");
        let other = Url::parse("https://getclickfix.com.evil.test/").expect("url");
        let http = Url::parse("http://getclickfix.com/").expect("url");
        assert!(c.is_app_origin(&ok));
        assert!(!c.is_app_origin(&other));
        assert!(!c.is_app_origin(&http));
    }
}
