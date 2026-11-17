# CF-235 ölçümler (16 Kas 2026, MacBook Air M2, macOS 27.2)

Yerel LiveKit (A ve B: compose.local.yml; C: aynı sürümde tek başına konteyner, Redis'siz), izleyici Chrome'da `harness/web/viewer.html`. Gecikme: ekrandaki saat ile videodaki saatin farkı (ekran görüntüsü).

| | A: Tauri WKWebView getDisplayMedia | B: Go sidecar (ScreenCaptureKit + VideoToolbox H.264) | C: Rust scap + livekit |
|---|---|---|---|
| Çalıştı mı | Evet, **yalnız WebKit gizli tercihleriyle** (`_setScreenCaptureEnabled`, `_setMediaDevicesEnabled`); varsayılan WKWebView'de `navigator.mediaDevices` yok | Evet | Evet (`build.rs` ile `-ObjC` bağlayıcı bayrağı gerekti) |
| Uçtan uca gecikme | ~133 ms | **~67 ms** | ~100 ms |
| Görüntü | 1710x1112, VP8, ~15 fps, 1,7 Mbps | 1660x1080, H.264 donanım, 25-29 fps, 0,5-2,5 Mbps | 1920x1248, VP8 yazılım, ~29 fps, 1,2 Mbps |
| CPU / bellek | %0,2-6 / 60-130 MB (uygulama + WebKit süreçleri) | %3-6 / 64 MB RSS (35 MB footprint) | ~%60 / 450-670 MB RSS |
| Paket / ikili boyutu | .app 3 MB | 29 MB (ek dylib yok) | 16 MB; derleme 9 dk, libwebrtc ~1,1 GB indirme |
| İzin | Sistem ekran seçicisi + Ekran Kaydı | Ekran Kaydı (uygulamaya) | Ekran Kaydı (uygulamaya) |

Girdi (`input-go`, robotgo + Erişilebilirlik izni): izleyicideki tıklama müşteri ekranında uygulandı (kullanıcı doğruladı). Gecikme, betikle `agent` kimliğinden gönderilen 60 olayla (20 tıklama çifti + 20 tuş) kuru modda ölçüldü: medyan 2 ms, p95 4 ms, en çok 21 ms (ilk olay), kayıp yok. Ölçüm ağsızdır; gerçek ağda RTT eklenir.

Karar: **B (Go sidecar)**. Gerekçe ve kalan riskler `docs/design/masaustu.md` (2. Kararlar, madde 5).

Denenmeyenler: 4K (bu ekranda yok), macOS'un ekran kaydı iznini aylık yeniden sorması, Windows (makine yok).
