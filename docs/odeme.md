# Ödeme runbook'u (iyzico / PayTR)

Kartla ödeme, kredi satın alma, deneme ödemesi ve abonelik yenilemesi için işletme notları (CF-154). Değer veya
sır içermez; sırlar sadece Dokploy ortam değişkenlerine girilir. Değişkenlerin tam listesi:
[ortam-degiskenleri.md](./ortam-degiskenleri.md).

## Akış özeti

1. Panel `GET /v1/tenant/billing/payment-availability` ile hangi yolun hazır olduğunu sorar. Hazır olmayan satın alma
   düğmeleri kapalıdır ve nedeni gösterilir.
2. `POST /v1/tenant/billing/credit-purchases` (veya plan/deneme faturası) sunucuda tutarı hesaplanmış bir fatura açar.
3. `POST /v1/tenant/billing/checkout` sağlayıcının ödeme sayfasını açar. Tutar her zaman faturadan gelir; istemcinin
   gönderdiği tutar kullanılmaz. Aynı diyalogdan gelen tekrar (aynı idempotency key) aynı sayfayı döndürür; açık bir
   sayfa varken yeni diyalog da aynı sayfaya yönlenir.
4. Sonuç sadece sağlayıcıdan doğrulanır: webhook (imzalı), iyzico dönüşü (API ile sorgu) veya 5 dakikada bir çalışan
   mutabakat işi (`ProviderSync`). Tarayıcıya dönen adres hiçbir zaman "ödendi" kanıtı değildir.
5. Ödeme başarılıysa aynı veritabanı işleminde fatura ödenir ve krediler deftere (`billing_credit_ledger`) bir kez
   yazılır (`purchase:<fatura>` anahtarı + `granted_at`).

Ödeme durumları ve izinli geçişler (veritabanı tetikleyicisi `trg_billing_payment_transition` zorlar):

| Durum | Anlamı | Geçebileceği durumlar |
|---|---|---|
| `pending` | Sayfa açıldı, sonuç yok | `unknown`, `succeeded`, `failed`, `expired` |
| `unknown` | Sağlayıcı çağrısının sonucu belirsiz | `succeeded`, `failed`, `expired` |
| `expired` | Sayfa 45 dakikada tamamlanmadı (terk edildi) | `succeeded`, `failed` (geç gelen sağlayıcı sonucu) |
| `succeeded`, `failed` | Kesin | yok |

İadeler ayrı `kind='refund'` satırıdır ve orijinal ödemeye bağlanır. Terk edilen bir sayfa geç ödenirse ödeme yine
işlenir; aynı fatura iki kez ödenmişse fazlası cüzdana TRY kredisi olarak yazılır.

## Gerekli ortam değişkenleri

Ortak (her iki sağlayıcı için):

| Değişken | Değer |
|---|---|
| `BILLING_SELLER_JSON` | Şirketin fatura kimliği, tek satır JSON (biçim `.env.example`). **Boşsa satın alma 409 `BILLING_SELLER_NOT_CONFIGURED` verir.** |
| `PUBLIC_API_URL` | `https://api.getclickfix.com` (iyzico dönüş adresi ve webhook'lar bu alan adında) |
| `PUBLIC_FRONTEND_URL` / `AUTH_URL` | Panel adresi; ödeme sonrası `/billing` sayfasına dönülür |
| `ENCRYPTION_KEY` | Zaten kurulu; webhook gövdeleri ve kart anahtarları bununla şifrelenir |

iyzico:

| Değişken | Değer |
|---|---|
| `PAYMENTS_IYZICO_ENABLED` | `true` |
| `PAYMENTS_IYZICO_MODE` | `sandbox` veya `live` |
| `PAYMENTS_IYZICO_API_KEY`, `PAYMENTS_IYZICO_SECRET_KEY` | iyzico panel → Ayarlar → Firma Ayarları (sandbox ve canlı anahtarları ayrıdır) |
| `PAYMENTS_IYZICO_ACCOUNT_UUID` | Boş bırakın (sağlayıcı + mod + API anahtarından türetilir). Anahtar değişince eski ödemelerin sorgulanabilmesi için eski UUID'yi girin; açılıştaki `billing_payments_ready` günlüğü sağlayıcıları listeler |
| `PAYMENTS_IYZICO_STATEMENTS_ENABLED` | Boş = açık (gece hesap özeti içe aktarma) |

PayTR:

| Değişken | Değer |
|---|---|
| `PAYMENTS_PAYTR_ENABLED` | `true` |
| `PAYMENTS_PAYTR_MODE` | `sandbox` (PayTR `test_mode=1`) veya `live` |
| `PAYMENTS_PAYTR_MERCHANT_ID`, `PAYMENTS_PAYTR_MERCHANT_KEY`, `PAYMENTS_PAYTR_MERCHANT_SALT` | PayTR Mağaza Paneli → Bilgi → API Entegrasyon Bilgileri |
| `PAYMENTS_PAYTR_ACCOUNT_UUID` | Boş bırakın (iyzico ile aynı kural) |

Birden fazla sağlayıcı açıksa varsayılan `PUT /v1/platform/billing/gateway-settings` (`default_provider`,
`currency_providers`) ile seçilir; bunun için henüz panel ekranı yok. Ayar boşsa veya seçilen sağlayıcı kapalıysa
sıradaki açık sağlayıcı kullanılır (iyzico, sonra PayTR). Tek sağlayıcı açıkken ayar gerekmez.

## Sandbox ve canlı

- `*_MODE=sandbox`: iyzico `sandbox-api.iyzipay.com`, PayTR `test_mode=1`. Gerçek para çekilmez; panel ödeme
  diyaloğunda "Test modu" uyarısı görünür.
- Canlıya geçiş: canlı anahtarları girin, `*_MODE=live` yapın, uygulamayı yeniden başlatın. Sandbox anahtarıyla canlı
  mod (veya tersi) sağlayıcı tarafından reddedilir; ödeme açılmaz (`502 PAYMENT_PROVIDER_REJECTED`) ve fatura kilitlenmez.
- Mod, bildirimdeki test bayrağıyla karşılaştırılır: canlı modda gelen test bildirimi (veya tersi) işlenmez.

## Sağlayıcı panelinde kaydedilecek adresler

| Sağlayıcı | Nereye | Adres |
|---|---|---|
| iyzico | iyzico panel → Ayarlar → Firma Ayarları → Bildirimler (Webhook / Merchant Notification) | `https://api.getclickfix.com/v1/public/billing/payments/iyzico/notify` |
| PayTR | Mağaza Paneli → Destek & Kurulum → Ayarlar → Bildirim URL | `https://api.getclickfix.com/v1/public/billing/payments/paytr/notify` |

Panel menü adları sağlayıcıya göre değişebilir. iyzico ödeme dönüş adresi (`callbackUrl`,
`/v1/public/billing/payments/iyzico/return/<referans>`) her istekte gönderilir, panelde kaydedilmez. PayTR başarılı /
başarısız dönüş adresleri de istekte gönderilir (`/billing?payment_ref=...`).

Webhook yanıtları:

| Kod | Ne zaman | Sağlayıcı tekrar dener mi |
|---|---|---|
| 200 | İşlendi, tekrar gelen (aynı olay bir kez uygulanır) veya imzası doğru ama bize ait olmayan sipariş | Hayır |
| 400 | İmza yanlış, gövde bozuk, tutar/para birimi/mod uyuşmuyor (mutabakat raporuna düşer) | Evet ama sonuç değişmez |
| 403 | Kaynak IP, CIDR listesi doluyken listede değil | Evet |
| 404 | Sağlayıcı bu ortamda kapalı | Evet |
| 413 / 429 | Gövde 64 KiB'tan büyük / dakikada 300'den fazla istek | Evet |
| 503 | Geçici iç hata (veritabanı, sağlayıcı API zaman aşımı) | Evet, güvenle |

Her webhook `billing_payment_notify` adıyla yapılandırılmış günlük satırı yazar (sağlayıcı, durum kodu, sonuç, kaynak
IP). Kart verisi, gövde veya kişisel veri günlüğe yazılmaz; ham gövde veritabanında şifreli saklanır.

## CIDR listeleri

- Varsayılan: `PAYMENTS_IYZICO_WEBHOOK_CIDRS` ve `PAYMENTS_PAYTR_WEBHOOK_CIDRS` boş. Her kaynaktan gelen istek kabul
  edilir; güvenlik sınırı sağlayıcı imzasıdır (iyzico `X-IYZ-SIGNATURE-V3`, PayTR `hash`; sabit zamanlı karşılaştırma).
  Sağlayıcıların IP aralıkları önceden haber verilmeden değişebildiği için boş bırakılması önerilir.
- Liste girilecekse aralıkları sağlayıcının destek ekibinden yazılı alın.
- `PAYMENTS_TRUSTED_PROXY_CIDRS` boşsa özel ağlar (10/8, 172.16/12, 192.168/16, loopback, fc00::/7) proxy sayılır ve
  gerçek kaynak `X-Forwarded-For`'dan okunur; API yalnızca Traefik/BFF üzerinden erişildiği için bu yeterlidir. Hız
  sınırı (bildirim 300/dk, dönüş 60/dk) bu kaynağa göre işler. Daraltmak için Dokploy sunucusunda
  `docker network inspect dokploy-network` çıktısındaki Subnet girilebilir.

## Canlı bir ödemeyi güvenle doğrulama

1. Önce sandbox: `*_MODE=sandbox` ile bir test organizasyonunda en küçük kredi paketini satın alın. Sağlayıcının test
   kartıyla ödeyin. Beklenen: `/billing` sayfasında "Ödemeniz doğrulanıyor" sonra "Ödeme alındı"; cüzdanda krediler.
2. API günlüğünde `billing_payments_ready` satırında sağlayıcının ve `seller_configured=true`'nun göründüğünü,
   ödemeden sonra `billing_payment_notify ... status=200 outcome=processed` satırını kontrol edin.
3. Canlıya geçin ve **küçük tutarlı** bir paketle (platform yöneticisi geçici, düşük tutarlı ve sadece test
   organizasyonunun planına açık bir paket tanımlayabilir) kendi kartınızla bir ödeme yapın.
4. Doğrulayın: Platform → Faturalama → Ödemeler sekmesinde (`/platform/billing?tab=payments`) ödeme `succeeded`, fatura `paid`; mutabakat raporunda
   uyuşmazlık yok; sağlayıcı panelinde aynı tutar.
5. Ödemeyi Platform → Faturalama → Ödemeler → İade ile iade edin (adım onayı ister). Kullanılmamış krediler geri alınır.
6. Test paketini pasifleştirin.

## Sık görülen hatalar

| Kod | Neden | Yapılacak |
|---|---|---|
| `BILLING_SELLER_NOT_CONFIGURED` (409) | `BILLING_SELLER_JSON` boş, organizasyonun önceki faturası yok | Değişkeni girin, yeniden başlatın |
| `PAYMENT_GATEWAY_NOT_CONFIGURED` (409) | Hiç sağlayıcı açık değil | `PAYMENTS_<P>_ENABLED=true` ve anahtarlar |
| `PAYMENT_PROVIDER_DISABLED` (409) | İstekte adı geçen sağlayıcı kapalı | Sağlayıcıyı açın ya da ödeme ayarlarını düzeltin |
| `PAYMENT_METHOD_UNAVAILABLE` (409) | Ne kart sağlayıcısı ne aktif banka hesabı var | Sağlayıcı açın veya Platform → Faturalama'dan banka hesabı ekleyin |
| `BILLING_PROFILE_INCOMPLETE` (422) | Müşterinin fatura bilgileri eksik | Müşteri `/billing` → Fatura bilgileri |
| `CHECKOUT_IN_PROGRESS` (409) | Faturanın otomatik tahsilatı sonuçlanmadı | Mutabakat işinin bitmesini bekleyin |
| `PAYMENT_PROVIDER_REJECTED` (502) | Sağlayıcı sayfa açmayı reddetti (yanlış anahtar, mod uyuşmazlığı) | Anahtarları ve `*_MODE`'u kontrol edin |
| `PAYMENT_PROVIDER_UNAVAILABLE` (503) | Sağlayıcı zaman aşımı | Geçicidir; aynı istek güvenle tekrarlanır |
