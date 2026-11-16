//! IPC izinleri. Uzak köke yalnız köprü komutları, yalnız ana pencerede ve
//! yalnız `CLICKFIX_APP_URL` kökü için açılır (`remote.urls = <kök>/*`).
//! Yerel sayfa (`capabilities/local.json`) yalnız giriş ve bağlantı
//! komutlarını çağırabilir. Eklentilerin (bildirim, kısayol, derin bağlantı,
//! opener...) JS komutları hiçbir pencereye açılmaz; onları yalnız Rust
//! tarafı kullanır.

use clickfix_shell::AppConfig;
use clickfix_shell::plugins::MAIN_WINDOW;
use tauri::ipc::CapabilityBuilder;

use crate::commands::REMOTE_COMMANDS;

pub const REMOTE_IDENTIFIER: &str = "remote-app";

/// Uygulama komutunun izin adı (`tauri-build` `AppManifest` kuralı).
pub fn permission(command: &str) -> String {
    format!("allow-{}", command.replace('_', "-"))
}

pub fn remote_permissions() -> Vec<String> {
    REMOTE_COMMANDS.iter().map(|c| permission(c)).collect()
}

pub fn remote(config: &AppConfig) -> CapabilityBuilder {
    remote_permissions().into_iter().fold(
        CapabilityBuilder::new(REMOTE_IDENTIFIER)
            .remote(config.remote_pattern())
            .window(MAIN_WINDOW),
        |b, p| b.permission(p),
    )
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::commands::LOCAL_COMMANDS;

    #[test]
    fn remote_gets_only_bridge_commands() {
        let perms = remote_permissions();
        assert_eq!(
            perms,
            [
                "allow-notify",
                "allow-set-badge",
                "allow-set-presence",
                "allow-open-external",
                "allow-version",
                "allow-sign-out"
            ]
        );
        // Çekirdek ve eklenti izinleri ("core:...", "notification:...") yok.
        assert!(perms.iter().all(|p| !p.contains(':')));
        // Giriş ve bağlantı komutları uzak köke açılmaz.
        for c in LOCAL_COMMANDS {
            assert!(!perms.contains(&permission(c)), "{c} uzak köke açık");
        }
    }

    #[test]
    fn remote_pattern_is_the_app_root_only() {
        let c = AppConfig::from_value(Some("https://staging.getclickfix.com/tr/x"))
            .expect("yapılandırma");
        assert_eq!(c.remote_pattern(), "https://staging.getclickfix.com/*");
    }

    #[test]
    fn local_capability_file_matches_local_commands() {
        let file: serde_json::Value =
            serde_json::from_str(include_str!("../capabilities/local.json")).expect("json");
        assert_eq!(file["local"], true);
        assert!(
            file.get("remote").is_none(),
            "yerel capability uzak kök almaz"
        );
        assert_eq!(file["windows"], serde_json::json!([MAIN_WINDOW]));
        let perms: Vec<String> = file["permissions"]
            .as_array()
            .expect("dizi")
            .iter()
            .map(|p| p.as_str().expect("metin").to_owned())
            .collect();
        let expected: Vec<String> = LOCAL_COMMANDS.iter().map(|c| permission(c)).collect();
        assert_eq!(perms, expected);
    }

    #[test]
    fn no_other_capability_files() {
        let dir = std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join("capabilities");
        let mut names: Vec<String> = std::fs::read_dir(dir)
            .expect("dizin")
            .map(|e| e.expect("girdi").file_name().to_string_lossy().into_owned())
            .collect();
        names.sort();
        assert_eq!(names, ["local.json"]);
    }
}
