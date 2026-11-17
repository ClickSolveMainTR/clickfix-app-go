include!("src/commands.rs");

fn main() {
    // Uygulama komutları yalnız capability ile açıkça izin verilen pencerelere
    // açılır (varsayılan "her yerel pencere" davranışı kapatılır).
    let commands: Vec<&'static str> = LOCAL_COMMANDS
        .iter()
        .chain(REMOTE_COMMANDS)
        .copied()
        .collect();
    let commands: &'static [&'static str] = Vec::leak(commands);
    tauri_build::try_build(
        tauri_build::Attributes::new()
            .app_manifest(tauri_build::AppManifest::new().commands(commands)),
    )
    .expect("tauri-build başarısız");
}
