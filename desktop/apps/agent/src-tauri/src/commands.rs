// Uygulama komutlarının tek listesi. `build.rs` bu dosyayı `include!` ile
// okur (AppManifest), `capability.rs` aynı listelerden capability üretir ve
// testler capability dosyalarıyla karşılaştırır. Bir komut eklerken yalnız
// burayı ve `lib.rs` içindeki `generate_handler!` listesini değiştirin.

/// Paketlenmiş yerel sayfanın (`ui/index.html`) çağırabildiği komutlar.
// Uygulamada yalnız testler okur; build.rs AppManifest'e verir.
#[allow(dead_code)]
pub const LOCAL_COMMANDS: &[&str] = &["connect", "start_login", "cancel_login"];

/// Web köprüsünün (`frontend/src/lib/desktop/bridge.ts`) uygulama kökünden
/// çağırabildiği komutlar. Başka hiçbir köke açılmaz.
pub const REMOTE_COMMANDS: &[&str] = &[
    "notify",
    "set_badge",
    "set_presence",
    "open_external",
    "version",
    "sign_out",
];
