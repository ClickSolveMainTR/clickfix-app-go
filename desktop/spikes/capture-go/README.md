# capture-go (CF-235, aday B)

Go yan süreçte yerel ekran yakalama denemesi. Ana ekranı **ScreenCaptureKit** (SCStream, NV12) ile yakalar, **VideoToolbox** donanım kodlayıcısıyla H.264'e (Constrained Baseline, B-kare yok, düşük gecikmeli hız denetimi) çevirir ve `customer-go` kimliğiyle LiveKit odasına `SCREEN_SHARE` izi olarak yayınlar. SCStream'in piksel tamponu kopyalanmadan kodlayıcıya gider; ölçekleme SCK içinde yapılır.

## Derleme

```sh
go build -o /tmp/cf235-capture-go .
go vet ./...
```

Gerekenler: Xcode CLT (macOS SDK), cgo. Homebrew kütüphanesi **gerekmez**; yalnız sistem framework'leri bağlanır (ScreenCaptureKit, VideoToolbox, CoreMedia, CoreVideo, CoreGraphics, AppKit). `go.mod` bağımlılıklar yüzünden `go 1.26.8` ister; Go araç zinciri kendini indirir.

Varsayılan hedef sürüm kurulu SDK'dır (27.2). Eski macOS için `MACOSX_DEPLOYMENT_TARGET=14.0 go build ...` (önce `go clean -cache`, yoksa bağlayıcı uyarı verir).

## Çalıştırma

Harness (`../harness`) ve yerel LiveKit açık olmalı. LiveKit'te `auto_create: false` olduğu için `cf235-spike` odası önceden oluşturulmalı (harness bunu yapmaz).

```sh
/tmp/cf235-capture-go -smoke                  # yalnız odaya bağlanır, izin istemez
/tmp/cf235-capture-go -synthetic -res 1080    # ekran yerine test deseni, izin istemez
/tmp/cf235-capture-go -res 1080 -fps 30       # gerçek yakalama
/tmp/cf235-capture-go -res native -bitrate 8000 -duration 60s
```

Bayraklar: `-res 1080|2160|native`, `-fps`, `-bitrate` (kbps, 0 = otomatik), `-harness`, `-identity`, `-duration`, `-smoke`, `-synthetic`.

Başlangıçta `CGPreflightScreenCaptureAccess()` denetlenir. İzin yoksa `CGRequestScreenCaptureAccess()` çağrılır ve program çıkar; izin terminal uygulamasına verilir ve terminal yeniden başlatılır.

Her 5 saniyede bir şunlar yazılır: yakalanan fps (yeni içerikli kare), boş kare/s, kodlanan fps, çıkış kbps, süreç CPU% (getrusage), RSS ve phys_footprint (mach task_info), anahtar kare sayısı, PLI/FIR sayısı, düşen kare.

## Bilinen sınırlar

- SCK ekran durağanken yeni kare göndermez; "yakalanan fps" ekrandaki harekete bağlıdır. Ölçümde ekranda hareket (video, kaydırma) olmalı.
- Bit hızı sabit hedeftir; ağ geri bildirimiyle (REMB/TWCC) ayarlanmaz. Simulcast yok.
- SDP `profile-level-id=42e01f` bildirir; 1440p/2160p akışın gerçek seviyesi daha yüksektir. Chromium sorunsuz çözdü (1080p ile denendi); Safari ve 2160p denenmedi.
- Yalnız ana ekran; çoklu ekran, imleç ayrı katmanı ve ses yok.
- `-synthetic` CPU değeri test desenini C'de doldurmanın maliyetini içerir, gerçek yakalamayla kıyaslanmaz.
