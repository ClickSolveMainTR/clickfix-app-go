fn main() {
    // Uygulama komutları yalnız capability ile açıkça izin verilen pencerelere
    // açılır (varsayılan "her yerel pencere" davranışı kapatılır).
    tauri_build::try_build(
        tauri_build::Attributes::new()
            .app_manifest(tauri_build::AppManifest::new().commands(&["sidecar_version"])),
    )
    .expect("tauri-build başarısız");
}
