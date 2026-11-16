# CF-235 ara ölçümler (16 Kas 2026, MacBook Air M2, macOS 27.2)

Yerel LiveKit (compose.local.yml), izleyici Chrome'da `harness/web/viewer.html`. Gecikme: ekrandaki saat ile videodaki saatin farkı (ekran görüntüsü).

| | A: Tauri WKWebView getDisplayMedia | B: Go sidecar (ScreenCaptureKit + VideoToolbox H.264) | C: Rust scap + livekit |
|---|---|---|---|
| Çalıştı mı | Evet, **yalnız WebKit gizli tercihleriyle** (`_setScreenCaptureEnabled`, `_setMediaDevicesEnabled`); varsayılan WKWebView'de `navigator.mediaDevices` yok | Evet | Derlendi; ilk çalıştırmada çöktü (`-ObjC` eksik), düzeltildi, henüz denenmedi |
| Uçtan uca gecikme | ~133 ms | **~67 ms** | - |
| Görüntü | 1710x1112, VP8, ~15 fps, 1,7 Mbps | 1660x1080, H.264 donanım, 25-29 fps, 0,5-2,5 Mbps | - |
| CPU / bellek | %0,2-6 / 60-130 MB (uygulama + WebKit süreçleri) | %3-6 / 64 MB RSS (35 MB footprint) | - |
| Paket / ikili boyutu | .app 3 MB | 29 MB (ek dylib yok) | 16 MB; derleme 9 dk, libwebrtc ~1,1 GB indirme |
| İzin | Sistem ekran seçicisi + Ekran Kaydı | Ekran Kaydı (uygulamaya) | Ekran Kaydı (uygulamaya) |

Girdi (`input-go`, robotgo + Erişilebilirlik izni): izleyicideki tıklama müşteri ekranında uygulandı (kullanıcı doğruladı); klavye olayı 85 ms'de ulaştı. Gecikme kaydı eksik: stdout ve stderr aynı dosyaya yazılınca satırlar kayboldu, yeniden ölçülecek.

Kalan: C'nin çalıştırılması, girdi gecikmesinin kaydı, 4K (bu ekranda yok), Sequoia aylık yeniden sorma, Windows (makine yok).
