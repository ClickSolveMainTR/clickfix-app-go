# capture-rust (CF-235, aday C)

Keşif programı: ana ekranı `scap` (ScreenCaptureKit) ile yakalar, BGRA kareleri
I420'ye çevirir (libwebrtc içindeki libyuv, `yuv_helper::argb_to_i420`) ve LiveKit
Rust SDK ile `customer-rust` kimliğiyle `Screenshare` kaynaklı bir video track olarak
yayınlar. Token, harness'tan (`GET /token?identity=customer-rust&publish=1`) alınır.
Her 5 saniyede yakalanan/gönderilen fps, toplam kare, CPU% ve RSS yazar.

Kendi `[workspace]` tablosu vardır; `desktop/Cargo.toml` workspace'ine dahil değildir
(oradaki `members` listesi açık yollar kullanır, glob yok).

## Derleme

```sh
export PATH="$(brew --prefix rustup)/bin:$PATH"
export CARGO_TARGET_DIR=/Volumes/DockerRuns/cargo-target/cf235
cd desktop/spikes/capture-rust
cargo build --release
```

## Çalıştırma

Önce harness (`:7890`) ve yerel LiveKit (`:7880`) açık olmalı.

```sh
B=/Volumes/DockerRuns/cargo-target/cf235/release/capture-rust
$B --check                 # destek/izin durumu; izin İSTEMEZ
$B --res 1080 --fps 30     # yayın (varsayılanlar bunlar)
$B --res 2160 --fps 30
$B --res native --fps 30 --harness http://127.0.0.1:7890
```

İzin yoksa program izin penceresini açar ve çıkar (kod 3). İzin, programı çalıştıran
uygulamaya (ör. Terminal) verilir; verdikten sonra programı yeniden başlatın.
Durdurmak için Ctrl+C.

## Sürümler

| Crate | Sürüm |
|-------|-------|
| scap | 0.1.0-beta.1 (en güncel; kararlı 0.0.x serisi eski) |
| livekit | 0.11.0 (libwebrtc 0.4.1, webrtc-sys 0.3.54) |
| ureq | 3.5.2 |
| sysinfo | 0.37.2 |
| tokio | 1.54.1 |

Rust 1.100.0, macOS 27.2, Apple M2 (arm64).

## Ölçümler (bu makinede)

- İlk (soğuk) `cargo build --release`: **9 dk 16 sn** (557 sn duvar saati), hazır
  libwebrtc arşivinin indirilmesi dahil.
- Release ikili (`opt-level = "s"`, `lto = true`, `strip = true`, `codegen-units = 1`):
  **16 MB** (`ls -lh`), tek dosya, sadece sistem framework'lerine bağlı.
- libwebrtc etkisi: derleme sırasında hazır `libwebrtc.a` indirilir (açılmış hali
  ~1,1 GB, `libwebrtc.a` tek başına 706 MB); hedef dizini toplam ~2,9 GB. İkili boyutun
  büyük kısmı statik bağlanan libwebrtc'dir.

`--check` çıktısı (izin verilmemiş durumda):

```
scap::is_supported   = true
scap::has_permission = false
ana ekran            = id 1 "Built-in Retina Display"
çıkış boyutu (--res 1080) = 1920x1248 @ 30 fps
Not: izin yok. Normal çalıştırmada izin istenir ve program çıkar.
```

## Bilinen sınırlar

- `scap` 0.1.0-beta.1 bir ön sürümdür; API değişebilir.
- `--res 2160` bu dahili ekranda `native` ile aynı çıkar (3420x2224), çünkü scap
  istenen genişliği ekranın piksel genişliğiyle sınırlar.
- Ekran değişmediğinde ScreenCaptureKit kare göndermez (scap 0x0 "boş kare" iletir);
  bu yüzden durağan ekranda yakalanan fps düşer. Program boş kareleri ayrıca sayar.
- Kare başına iki kopya var: scap BGRA'yı satır satır yeni bir `Vec`'e kopyalar, sonra
  I420'ye çeviriyoruz. scap'in NV12 (`YUVFrame`) yolu daha ucuz olabilirdi ama bu
  sürümde chroma stride/yükseklik hesabı hatalı görünüyor, o yüzden BGRA kullanıldı.
- scap macOS'ta piksel tamponunun kilidini (`lock_base_addr`) açmıyor; uzun
  çalışmalarda bellek davranışı izlenmeli (RSS çıktısı bunun için).
- Tek ekran (ana ekran), imleç dahil, ses yok, simulcast kapalı, VP8.
- CPU% `sysinfo`'dan gelir; tek çekirdek = %100.
- Yalnızca `--check`/`--help` ile denendi; gerçek yakalama kullanıcı izniyle
  koordinatör tarafından çalıştırılacak.
