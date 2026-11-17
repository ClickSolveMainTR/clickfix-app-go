# input-go (CF-235, girdi enjeksiyonu)

Tüm yakalama adaylarıyla birlikte kullanılan girdi denemesi. `customer-input` kimliğiyle LiveKit odasına katılır, `control` konusundaki veri mesajlarını dinler ve **robotgo** ile ana ekrana uygular:

- `move`: normalize x,y (0..1) ana ekranın **nokta** boyutuna çevrilir (robotgo macOS'ta nokta kullanır; Retina ölçeği gerekmez). Tuş basılıyken `LeftMouseDragged`/`RightMouseDragged` olayı gönderilir (robotgo yalnız `MouseMoved` gönderdiği için sürükleme bozulmasın diye).
- `down`/`up`: önce imleç taşınır, sonra `robotgo.Toggle` (b: 0 sol, 1 orta, 2 sağ).
- `key`: Enter, Backspace, Tab, Escape, oklar, boşluk, Delete, Home/End, PageUp/PageDown adla; küçük harf ve rakam `KeyTap`; büyük harf `KeyTap(harf, "shift")`; diğer tek karakterler (noktalama, Türkçe harfler) `TypeStr`.
- `ts` varsa her olay `gecikme = şimdi - ts` ile loglanır. `move` mesajları yalnız `-dry` modda loglanır.

Varsayılan olarak yalnız `agent` kimliğinden gelen mesajlar uygulanır (`-from`).

## Derleme

```sh
go build -o /tmp/cf235-input-go .
go vet ./...
```

Gerekenler: Xcode CLT, cgo. Homebrew kütüphanesi gerekmez. robotgo v1.3.7 bu SDK'da (27.2) derlenir; kendi yakalama kodu için ScreenCaptureKit'i de bağlar (kullanılmıyor).

## Çalıştırma

```sh
/tmp/cf235-input-go -smoke     # odaya bağlanıp çıkar, izin denetlemez
/tmp/cf235-input-go -dry       # enjekte etmez, yalnız loglar; izin penceresi açmaz
/tmp/cf235-input-go            # gerçek enjeksiyon
```

Gerçek modda `AXIsProcessTrustedWithOptions` (prompt açık) çağrılır. İzin yoksa Türkçe mesaj yazılır ve çıkılır; izin Sistem Ayarları > Gizlilik ve Güvenlik > Erişilebilirlik'te terminal uygulamasına verilir.

## Bilinen sınırlar

- İzleyici yalnız `keydown` gönderir, değiştirici tuş (Cmd/Ctrl/Alt) bilgisi yok; kısayollar (Cmd+C vb.) çalışmaz.
- Çift tıklama tıklama sayısı (clickState) ayarlanmaz; iki hızlı tıklama macOS'ta çift tıklama sayılmayabilir.
- Kaydırma (wheel) mesajı yok.
- Yalnız ana ekran. Güvenli giriş alanları (parola, kilit ekranı) sentetik olayları reddeder.
