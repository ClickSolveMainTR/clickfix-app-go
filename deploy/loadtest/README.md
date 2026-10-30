# Yük testleri (k6)

CF-131. [k6](https://k6.io) senaryoları Go API'sine (`/v1`) doğrudan gider. Next BFF'den geçmezler; mobil
uygulamanın kullandığı Bearer token akışını kullanırlar. Tüm istekler **demo organizasyonunda** çalışır
([docs/demo-org.md](../../docs/demo-org.md)). Gerçek bir müşteri organizasyonuna karşı çalıştırılmaz.

| Script | Ölçtüğü | Eşik (p95) |
|--------|---------|------------|
| `inbox-list.js` | Konuşma listesi (farklı filtreler) + sayaçlar | liste < 500 ms, sayaçlar < 300 ms |
| `conversation-open.js` | Konuşma detayı + mesaj geçmişi + okundu işareti | detay < 400 ms, mesajlar < 500 ms |
| `reply.js` | Temsilci yanıtı (**iç not** olarak; müşteriye gitmez) | < 800 ms |
| `widget-message.js` | Web widget: bootstrap, yeni konuşma, ek mesaj, mesajları yoklama | bootstrap < 400 ms, gönderim < 700 ms |
| `search.js` | Cmd+K araması (`/v1/tenant/search`) | < 400 ms |

Her script'te ayrıca `http_req_failed < %1` ve `checks > %99` eşikleri vardır. Bir eşik aşılırsa k6 sıfır olmayan
kodla çıkar.

## Hazırlık

1. k6 kurun (`brew install k6` veya resmi Docker imajı `grafana/k6`).
2. Hedef ortamda demo organizasyonunu yenileyin ve çıktıdaki şifreyi ve widget kimliğini not edin:
   `make seed-demo` (yerel) ya da backend konteynerinde `/app/seed-demo` (staging / prod). `search.js` için
   ardından `search-reindex` çalıştırın.
3. Şifreyi ortam değişkeni olarak verin. Dosyaya veya komut geçmişine yazmamak için `read -s DEMO_PASSWORD` gibi bir
   yöntem kullanın.

## Çalıştırma

```sh
cd deploy/loadtest
export API_URL=http://127.0.0.1:8080/v1          # yerel; staging/prod: https://api.<alan-adı>/v1
read -s DEMO_PASSWORD && export DEMO_PASSWORD
k6 run inbox-list.js
k6 run conversation-open.js
k6 run reply.js
k6 run search.js
WIDGET_ID=<seed-demo çıktısındaki widget public id> k6 run widget-message.js
```

| Değişken | Varsayılan | Açıklama |
|----------|------------|----------|
| `API_URL` | `http://127.0.0.1:8080/v1` | Go API kökü |
| `DEMO_EMAIL` | `burak.sahin@anadolu-yazilim.example.com` | Demo temsilcisi |
| `DEMO_PASSWORD` | — | seed-demo çıktısı (zorunlu) |
| `ORG_SLUG` | `demo-anadolu-yazilim` | `X-Organization` başlığı |
| `VUS` | `5` | Eş zamanlı sanal kullanıcı |
| `RAMP` / `DURATION` | `30s` / `1m` | Isınma ve sabit yük süresi |
| `WIDGET_ID` | — | `widget-message.js` için zorunlu |
| `WIDGET_ORIGIN` | — | Widget'ın izinli alan adları varsa `Origin` başlığı |

## Staging / production'da güvenli çalıştırma

- **Düşük başlayın.** Varsayılan 5 sanal kullanıcı ve 2 dakikadır. Önce `VUS=1 DURATION=30s` ile duman testi
  yapın. Sonra yükü adım adım artırın (5 → 10 → 25). Her adımda Grafana panolarını izleyin
  ([docs/OBSERVABILITY.md](../../docs/OBSERVABILITY.md)): API p95, Postgres bağlantıları, Redis ve Asynq kuyruk
  derinliği.
- **Yoğun saatlerde çalıştırmayın.** Production'da mesai dışını seçin ve testi ekibe önceden duyurun.
- **Yazma senaryoları demo verisini büyütür.** `reply.js` iç not, `widget-message.js` her turda yeni konuşma yazar.
  Test bitince demo organizasyonunu `seed-demo` ile yenileyin; yenileme konuşmaları ve notları siler.
- **Giriş uç noktası test edilmez.** `setup()` çalıştırma başına bir kez giriş yapar. Giriş uç noktasının hız
  sınırını yük testine çevirmeyin.
- **Erişim jetonu 15 dakika geçerlidir.** Daha uzun koşular için `DURATION` değerini 12 dakikanın altında tutun ya
  da senaryoyu bölün.
- **Hız sınırları.** Widget, giriş ve herkese açık yardım merkezi uçlarında hız sınırı vardır. Tek makineden yüksek VU'da `429`
  yanıtları görürseniz bu bir performans sorunu değildir; sınırı ölçüyorsunuz demektir. Sonuçları buna göre
  okuyun.
- **Durdurma.** `Ctrl+C` k6'yı durdurur. Eşik aşımında senaryoyu hemen kesmek için `--abort-on-fail` ekleyebilir
  ya da eşiğe `abortOnFail` tanımlayabilirsiniz.

## Sonuçlar

`k6 run --summary-export=sonuc.json <script>` özetini dosyaya yazar. Bulguları ilgili CF maddesine yorum olarak
ekleyin. Şifre, token ve müşteri verisi eklemeyin.
