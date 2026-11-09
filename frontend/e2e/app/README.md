# Uçtan uca testler (CF-131)

Lansman için kritik akışları gerçek yığına karşı (Go API + `next start` + compose altyapısı) Playwright ile test
eder. Veri, demo organizasyonundan gelir ([docs/demo-org.md](../../../docs/demo-org.md)). Testler tek tarayıcıda ve
tek worker ile koşar.

| Spec                       | Akış                                                                                                                                                                                                                                                                                               |
| -------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `auth.setup.ts`            | Demo sahibi ve platform admini girişi (oturumlar `.auth/` altına yazılır), self-serve kaydı açar, sahibin modül rehberi otomatik açılışını kapatır                                                                                                                                                 |
| `signup-onboarding.e2e.ts` | Hatalı şifre; `/register` hesap + şirket → e-posta kodu → organizasyon/deneme → `/getting-started` → `/setup` kurulum sihirbazı (kanal, ekip, widget, ilk konuşma) → ikinci organizasyon → org değiştirici                                                                                         |
| `inbox.e2e.ts`             | Widget konuşması → gelen kutusu → kendine ata → yanıt (ziyaretçi widget'ta görür) → çöz; makro; kayıtlı görünüm; toplu çöz + geri al                                                                                                                                                               |
| `team.e2e.ts`              | Ekip arkadaşı daveti                                                                                                                                                                                                                                                                               |
| `knowledge.e2e.ts`         | Makale yayınla → herkese açık yardım merkezinde görünür; iç makale görünmez                                                                                                                                                                                                                        |
| `billing.e2e.ts`           | Deneme planı, limitler, herkese açık fiyat listesi (ödeme başlatmaz)                                                                                                                                                                                                                               |
| `platform.e2e.ts`          | Platform admini: organizasyonlar, kullanıcılar, sistem sağlığı                                                                                                                                                                                                                                     |
| `kvkk.e2e.ts`              | KVKK dışa aktarma başvurusu tamamlanır ve dosya indirilir (henüz ekranı yok, API üzerinden)                                                                                                                                                                                                        |
| `contacts.e2e.ts`          | Kişi listesi, yeni kişi, aramayla bulup açma                                                                                                                                                                                                                                                       |
| `widget.e2e.ts`            | Widget düzenleyicisi (taslak, yayın, geçmiş, kurulum) ve sayfaya gömülü widget                                                                                                                                                                                                                     |
| `guides.e2e.ts`            | Rehber ilerlemesi `PUT .../guides/{key}/progress` BFF üzerinden 2xx döner (API 204; PR #169 gerekir)                                                                                                                                                                                               |
| `video.e2e.ts`             | Görüntülü görüşme: kullanılamama mesajı, PIN'siz ve PIN zorunlu oda, misafir sayfası (PIN, bekleme odası, biten oda). LiveKit sunucusu gerekmez                                                                                                                                                    |
| `booking.e2e.ts`           | Herkese açık randevu sayfası: saat seç, randevu al, yönetim bağlantısından iptal                                                                                                                                                                                                                   |
| `notifications.e2e.ts`     | Uygulama içi bildirim zildeki listede görünür, tümü okundu işaretlenir                                                                                                                                                                                                                             |
| `security.e2e.ts`          | SCIM token bir kez gösterilir ve IdP olarak çalışır; API anahtarı oluştur, public API ile kullan, iptal et                                                                                                                                                                                         |
| `mcp-oauth.e2e.ts`         | MCP OAuth keşfi (`/.well-known/oauth-authorization-server`, `oauth-protected-resource/mcp/business`), istemci kaydı doğrulaması                                                                                                                                                                    |
| `assistant.e2e.ts`         | ClickFix Asistan (CF-183): REST API kanalı kurulur, imza anahtarı bir kez gösterilir, kanal test edilir; cümleden iş akışı (kuru çalıştır, onayla); sayfa durumu; "sayfaya git ve vurgula". API `ASSISTANT_E2E_FAKE_AI=1` ile senaryo oynatan sahte sağlayıcıyı kullanır (yalnız development/test) |

## Yerelde çalıştırma

```sh
make infra && make migrate-up
make create-super-admin SA_EMAIL=platform-admin@example.com SA_PASSWORD='E2eAdmin2026'
DEMO_PASSWORD='E2eDemo2026x' make seed-demo ARGS="-dev-catalog"
# Platform kayıt şekli "Kayıt açık" olmalı (auth.setup.ts bunu açar); sonra API'yi ve panelin production build'ini başlatın:
ASSISTANT_E2E_FAKE_AI=1 make backend-dev   # ya da backend/bin/api; bayrak asistan senaryoları için
cd frontend && pnpm build && pnpm exec next start -H 127.0.0.1 -p 3000

E2E_DEMO_PASSWORD='E2eDemo2026x' \
E2E_ADMIN_EMAIL=platform-admin@example.com E2E_ADMIN_PASSWORD='E2eAdmin2026' \
E2E_API_URL=http://127.0.0.1:8080 \
pnpm e2e                              # tek dosya: pnpm e2e inbox
```

`E2E_BASE_URL` varsayılan olarak `http://127.0.0.1:3000`, `E2E_API_URL` (API anahtarıyla public API ve MCP OAuth
keşfi doğrudan Go API'ye gider) `http://127.0.0.1:8080` adresidir. Hata durumunda iz (trace) ve ekran görüntüsü
`e2e/app/out/` altına yazılır: `pnpm exec playwright show-trace e2e/app/out/<test>/trace.zip`.

Testler her çalıştırmada benzersiz e-posta ve adlar üretir; aynı veritabanında tekrar çalıştırılabilir. Demo verisini
sıfırlamak için `make seed-demo` komutunu yeniden çalıştırın.

## Yazım kuralları

- `test` ve `expect`'i `./fixtures`'tan alın. Bu `test`, açılan modül rehberi turlarını geçer ve sayfada React
  hydration ya da liste anahtarı (`key`) hatası konsola düşerse testi düşürür.
- Arayüzün konu olmadığı yerde `page.request` / `request` ile API düzeyinde doğrulayın; seçicilerde rol ve etiket
  kullanın.

## CI

`.github/workflows/e2e.yml` elle tetiklenir (`workflow_dispatch`) ve her gece çalışır. Zorunlu `ci` kontrolünün
parçası değildir. Hata olursa `e2e-traces` artifact'i iz dosyalarını, HTML raporunu ve sunucu loglarını içerir;
başarısız testlerin sayfa görüntüsü (`error-context.md`) iş logunda da basılır.
