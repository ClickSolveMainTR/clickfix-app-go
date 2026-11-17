// libwebrtc'nin Objective-C kategorileri statik bağlamada yüklensin diye macOS'ta -ObjC gerekir.
fn main() {
    if std::env::var("CARGO_CFG_TARGET_OS").as_deref() == Ok("macos") {
        println!("cargo:rustc-link-arg=-ObjC");
    }
}
