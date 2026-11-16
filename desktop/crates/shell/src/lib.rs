//! ClickFix masaüstü uygulamalarının (ClickFix Masaüstü ve ClickFix Destek)
//! ortak Rust kabuğu.
//!
//! - [`sidecar`]: Go sidecar'ı başlatır, `ping` ile doğrular, çökerse geri
//!   çekilmeli olarak yeniden başlatır.
//! - [`rpc`]: sidecar ile stdin/stdout satır sınırlı JSON-RPC istemcisi.
//! - [`config`]: ortam değişkenlerinden yapılandırma.
//! - [`net`]: uzak köke erişilebilirlik denetimi.
//! - [`plugins`]: tek örnek ve günlük dosyası eklentileri.
//! - [`updates`]: Tauri updater ile kendini güncelleme (kapanışta kurulum).

pub mod backoff;
pub mod config;
pub mod net;
pub mod plugins;
pub mod rpc;
pub mod sidecar;
pub mod updates;

pub use backoff::Backoff;
pub use config::AppConfig;
pub use rpc::{CallError, Event};
pub use sidecar::{Sidecar, SidecarSpec};

/// Sidecar ikilisinin paketteki temel adı (`bundle.externalBin`).
pub const SIDECAR_NAME: &str = "clickfix-sidecar";
