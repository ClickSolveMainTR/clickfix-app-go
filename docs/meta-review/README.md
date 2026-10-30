# Meta App Review: videolar ve açıklama metinleri

ClickFix Meta uygulaması (App ID `1117894644223620`) App Review'a giderken her kullanım durumu için bir ekran kaydı
ve her izin için İngilizce bir açıklama ister. Bu klasör o metinleri, `frontend/e2e/meta-review/` ise videoları
üreten Playwright senaryolarını (kompozisyonları) tutar. Meta tarafındaki kurulum: [../meta-kurulum.md](../meta-kurulum.md).

| Kullanım durumu | Açıklama metni | Video (senaryo) | İzinler |
|-----------------|----------------|-----------------|---------|
| Messenger from Meta | [messenger.md](./messenger.md) | `messenger.review.ts` | `pages_show_list`, `pages_manage_metadata`, `pages_messaging`, `pages_read_engagement`, Human Agent |
| Instagram API | [instagram.md](./instagram.md) | `instagram.review.ts` | `instagram_business_basic`, `instagram_business_manage_messages`, `instagram_business_manage_comments` |
| Connect on WhatsApp | [whatsapp.md](./whatsapp.md) | `whatsapp.review.ts` | `whatsapp_business_management`, `whatsapp_business_messaging`, `business_management` |
| Uygulama geneli | [app-verification.md](./app-verification.md) | - | Gözden geçirenin girişi, platform notları |

## Videolar nasıl üretilir

Senaryolar gerçek Next.js panelini (İngilizce arayüz) sahte bir Go API'ye (`frontend/e2e/meta-review/mock-api.ts`)
karşı sürer: giriş, kanal bağlama, gelen mesaj ve yanıt adımları tekrarlanabilir olur, gerçek hesap veya sır gerekmez.
Her adımda ekranın üstünde İngilizce bir açıklama şeridi görünür; aynı metinler zaman damgasıyla `.vtt` altyazı
dosyasına yazılır.

```bash
cd frontend
pnpm review:dry-run      # tarayıcı açmadan senaryo, altyazı ve süre kontrolü
pnpm review:build && pnpm review:videos   # yerelde 1920x1080 (REVIEW_SIZE ile değişir) → e2e/meta-review/out/<kullanım-durumu>.{webm,mp4,vtt}
```

CI: GitHub Actions → **Meta review videos** (`.github/workflows/meta-review-videos.yml`, sadece elle tetiklenir).
Çıktılar `meta-review-videos` artifact'inde. Workflow hiçbir sır kullanmaz.

## Meta'nın kendi ekranları

Facebook / Instagram giriş ve izin ekranları Meta'ya aittir; CI'da otomatik sürülemez (2FA, CAPTCHA). Senaryo,
bağlama düğmesine basıldığı anı ve Meta'dan geri dönüşü (redirect) gösterir; arada Meta onay penceresini bir altyazı
anlatır. Gözden geçiren gerçek onay ekranını görmek isterse, test hesabıyla QuickTime kaydı alınıp ilgili videonun
"Connect" adımından sonra eklenir (bkz. her metindeki "Manual clip" notu).

## Gönderim

App Review formunu gönderme kararı kullanıcınındır. Bu metinler forma olduğu gibi yapıştırılmak için yazıldı.
